package fwd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mongodb/mongo-go-driver/bson"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/replaceopt"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

type channelList map[telepathy.Channel]bool

// PlainTable is the fwd table type used in Database
type PlainTable map[telepathy.Channel][]telepathy.Channel

// DBEntry is the type to be stored in the DB

type forwardingManager struct {
	session *telepathy.Session
	sync.Mutex
	table *sync.Map
}

func init() {
	telepathy.RegisterMessageHandler(msgHandler)
	telepathy.RegisterResource(funcKey, resourceCtor)
}

func resourceCtor(s *telepathy.Session) (interface{}, error) {
	manager := &forwardingManager{
		session: s,
		table:   &sync.Map{},
	}
	<-manager.loadFromDB()
	return manager, nil
}

func manager(s *telepathy.Session) *forwardingManager {
	load, ok := s.Resrc.Load(funcKey)
	if !ok {
		logger.Error("failed to load manager")
		return &forwardingManager{table: &sync.Map{}}
	}
	ret, ok := load.(*forwardingManager)
	if !ok {
		logger.Errorf("resource type error: %v", load)
		// Return a dummy manager to keeps things going
		// But it wont work well for sure
		return &forwardingManager{table: &sync.Map{}}
	}
	return ret
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
	logger.Infof("Fwd Created: %s -> %s", from.Name(), to.Name())
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
	retCh := make(chan interface{})
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			logger := logger.WithField("phase", "db")
			logger.Info("Start write-back to DB")
			collection := db.Collection(funcKey)
			doc := tableToBSON(m.table)
			m.Lock()
			defer m.Unlock()
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"type": "fwdtable"}, doc,
				replaceopt.Upsert(true))
			if err != nil {
				logger.Error("Error when write-back to DB: " + err.Error())
			}
			logger.Infof("write-back to DB, Done: result=%v", result)
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (m *forwardingManager) loadFromDB() chan interface{} {
	retCh := make(chan interface{})
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			logger := logger.WithField("phase", "db")
			logger.Info("Start loading from DB")
			collection := db.Collection(funcKey)
			m.Lock()
			defer m.Unlock()
			result := collection.FindOne(ctx, map[string]string{"type": "fwdtable"})
			doc := bson.NewDocument()
			err := result.Decode(doc)
			if err != nil {
				logger.Error("Error when load from DB: " + err.Error())
			} else {
				m.table = bsonToTable(doc)
			}
			logger.Info("load from DB, Done")
			return result
		},
		Return: retCh,
	})
	return retCh
}

func msgHandler(ctx context.Context, t *telepathy.Session, message telepathy.InboundMessage) {
	manager := manager(t)
	toChList := manager.forwardingTo(message.FromChannel)
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
			if len(message.Text) != 0 || message.Image.Length != 0 {
				outMsg.Text = text
			}
			msgr, _ := t.Msgr.Messenger(toCh.MessengerID)
			msgr.Send(outMsg)
		}
	}
}
