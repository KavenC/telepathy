package fwd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/replaceopt"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const dbTableName = "fwdtable"

type channelList map[telepathy.Channel]bool

type forwardingManager struct {
	telepathy.ServicePlugin
	session *telepathy.Session
	sync.Mutex
	table   *sync.Map
	context context.Context
	logger  *logrus.Entry
}

func init() {
	telepathy.RegisterService(funcKey, ctor)
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	manager := &forwardingManager{
		session: param.Session,
		logger:  param.Logger,
		table:   &sync.Map{},
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

func createFwdNoLock(from, to *telepathy.Channel, target *sync.Map) bool {
	newTo := channelList{*to: true}
	load, ok := target.LoadOrStore(*from, newTo)
	if ok {
		existsToList, _ := load.(channelList)
		if existsToList[*to] {
			return false
		}
		existsToList[*to] = true
		target.Store(*from, existsToList)
	}
	return true
}

func tableToBSON(table *sync.Map) *bson.Document {
	doc := bson.NewDocument(bson.EC.String("type", "fwdtable"))
	table.Range(func(key interface{}, value interface{}) bool {
		from, _ := key.(telepathy.Channel)
		toList, _ := value.(channelList)
		bsonToList := bson.NewArray()
		for to := range toList {
			bsonToList.Append(bson.VC.String(to.JSON()))
		}
		doc.Append(bson.EC.Array(from.JSON(), bsonToList))
		return true
	})
	return doc
}

func bsonToTable(doc *bson.Document) *sync.Map {
	table := &sync.Map{}
	eleIter := doc.Iterator()
	for {
		if !eleIter.Next() {
			break
		}
		element := eleIter.Element()
		toList, ok := element.Value().MutableArrayOK()
		if ok {
			fromCh := &telepathy.Channel{}
			json.Unmarshal([]byte(element.Key()), fromCh)
			toIter, _ := toList.Iterator()
			for {
				if !toIter.Next() {
					break
				}
				toCh := &telepathy.Channel{}
				jsonStr := toIter.Value().StringValue()
				json.Unmarshal([]byte(jsonStr), toCh)
				createFwdNoLock(fromCh, toCh, table)
			}
		}
	}
	return table
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

func (m *forwardingManager) createForwarding(from, to telepathy.Channel) bool {
	m.Lock()
	defer m.Unlock()
	return createFwdNoLock(&from, &to, m.table)
}

func (m *forwardingManager) removeForwarding(from, to telepathy.Channel) bool {
	m.Lock()
	defer m.Unlock()
	load, ok := m.table.Load(from)
	if !ok {
		return false
	}
	toList, _ := load.(channelList)
	if toList[to] {
		delete(toList, to)
		m.table.Store(from, toList)
		return true
	}
	return false
}

func (m *forwardingManager) forwardingTo(from telepathy.Channel) channelList {
	load, ok := m.table.Load(from)
	if !ok {
		return nil
	}
	ret, _ := load.(channelList)
	return ret
}

func (m *forwardingManager) forwardingFrom(to telepathy.Channel) channelList {
	ret := make(channelList)
	m.table.Range(func(key, value interface{}) bool {
		toList, _ := value.(channelList)
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
			doc := tableToBSON(m.table)
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"type": dbTableName}, doc,
				replaceopt.Upsert(true))
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
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			m.Lock()
			defer m.Unlock()
			result := collection.FindOne(ctx, map[string]string{"type": dbTableName})
			doc := bson.NewDocument()
			err := result.Decode(doc)
			if err != nil {
				m.logger.Error("error when loading table from DB: " + err.Error())
			} else {
				m.table = bsonToTable(doc)
			}
			m.logger.Info("load from DB done")
			return result
		},
		Return: retCh,
	})
	return retCh
}
