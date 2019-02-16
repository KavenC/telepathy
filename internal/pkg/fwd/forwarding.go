package fwd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const dbTableName = "fwdtable"

type forwardingManager struct {
	telepathy.ServicePlugin
	session *telepathy.Session
	table   *telepathy.ChannelListMap
	context context.Context
	logger  *logrus.Entry
}

func init() {
	telepathy.RegisterService(funcKey, ctor)
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	manager := &forwardingManager{
		session: param.Session,
		table:   telepathy.NewChannelListMap(),
		logger:  param.Logger,
	}
	manager.session.Message.RegisterMessageHandler(manager.msgHandler)
	return manager, nil
}

func (m *forwardingManager) Start(context context.Context) {
	m.context = context
	// Load forwarding table from DB
	<-m.loadFromDB()
}

func (m *forwardingManager) ID() string {
	return funcKey
}

func (m *forwardingManager) msgHandler(ctx context.Context, message telepathy.InboundMessage) {
	toChList := m.forwardingTo(message.FromChannel)
	if toChList != nil {
		text := fmt.Sprintf("**[ %s | %s ]**\n%s",
			message.FromChannel.MessengerID,
			message.SourceProfile.DisplayName,
			message.Text)
		for toCh := range toChList {
			outMsg := &telepathy.OutboundMessage{
				TargetID: toCh.ChannelID,
				Image:    message.Image,
			}
			if len(message.Text) != 0 || message.Image != nil {
				outMsg.Text = text
			}
			msgr, _ := m.session.Message.Messenger(toCh.MessengerID)
			msgr.Send(outMsg)
		}
	}
}

func (m *forwardingManager) forwardingTo(from telepathy.Channel) telepathy.ChannelList {
	load, ok := m.table.GetList(from)
	if !ok {
		return nil
	}
	return load
}

func (m *forwardingManager) forwardingFrom(to telepathy.Channel) telepathy.ChannelList {
	ret := make(telepathy.ChannelList)
	m.table.Range(func(key interface{}, toList telepathy.ChannelList) bool {
		if toList[to] {
			from, _ := key.(telepathy.Channel)
			ret[from] = true
		}
		return true
	})

	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (m *forwardingManager) writeToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			result, err := m.table.StoreToDB(ctx, collection, dbTableName)
			if err != nil {
				m.logger.Error("error when writing table back to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (m *forwardingManager) loadFromDB() chan interface{} {
	m.table = telepathy.NewChannelListMap()
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			err := m.table.LoadFromDB(ctx, collection, dbTableName, reflect.TypeOf(telepathy.Channel{}))
			if err != nil {
				m.logger.Errorf("load from DB failed: %s", err.Error())
				return err
			}
			m.logger.Info("load from DB done")
			return nil
		},
		Return: retCh,
	})
	return retCh
}
