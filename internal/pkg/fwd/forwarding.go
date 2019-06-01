package fwd

import (
	"context"
	"fmt"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const dbTableName = "fwdtable"

type forwardingManager struct {
	telepathy.ServicePlugin
	session *telepathy.Session
	table   table
	context context.Context
	logger  *logrus.Entry
}

func init() {
	telepathy.RegisterService(funcKey, ctor)
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	manager := &forwardingManager{
		session: param.Session,
		table:   table{logger: param.Logger.WithField("component", "table")},
		logger:  param.Logger,
	}
	manager.session.Message.RegisterMessageHandler(manager.msgHandler)
	return manager, nil
}

func (m *forwardingManager) Start(ctx context.Context) {
	m.context = ctx
	// Load forwarding table from DB
	err := m.loadFromDB()
	if err != nil {
		m.logger.Errorf("Table LoadDB failed: %s", err.Error())
	}
	m.table.start(m.context)
}

func (m *forwardingManager) ID() string {
	return funcKey
}

func (m *forwardingManager) msgHandler(ctx context.Context, message telepathy.InboundMessage) {
	toChList := m.forwardingTo(message.FromChannel)
	if toChList != nil {
		for toCh, alias := range toChList {
			outMsg := &telepathy.OutboundMessage{
				TargetID: toCh.ChannelID,
				Image:    message.Image,
			}
			text := fmt.Sprintf("**[ %s | %s ]**\n%s",
				alias.SrcAlias,
				message.SourceProfile.DisplayName,
				message.Text)
			if len(message.Text) != 0 || message.Image != nil {
				outMsg.Text = text
			}
			msgr, _ := m.session.Message.Messenger(toCh.MessengerID)
			msgr.Send(outMsg)
		}
	}
}

func (m *forwardingManager) forwardingTo(from telepathy.Channel) channelList {
	return m.table.getTo(from)
}

func (m *forwardingManager) forwardingFrom(to telepathy.Channel) channelList {
	return m.table.getFrom(to)
}

func (m *forwardingManager) writeToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	bsonChan := m.table.bson()
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			tableBSON := bson.M{"ID": dbTableName, "Table": *(<-bsonChan)}
			collection := db.Collection(funcKey)
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"ID": dbTableName}, tableBSON, options.Replace().SetUpsert(true))
			if err != nil {
				m.logger.Error("error when writing table back to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (m *forwardingManager) loadFromDB() error {
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			result := collection.FindOne(ctx, map[string]string{"ID": dbTableName})
			raw, err := result.DecodeBytes()
			if err != nil {
				return err
			}
			return raw.Lookup("Table")
		},
		Return: retCh,
	})

	// Wait until DB operation is done
	result := <-retCh
	if err, ok := result.(error); ok {
		return err
	}

	bsonValue, _ := result.(bson.RawValue)
	return m.table.loadBSON(bsonValue)
}
