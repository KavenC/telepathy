package plurkrss

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/replaceopt"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const timeZone = "Asia/Taipei"
const dbType = "plurksubtable"

type channelList map[telepathy.Channel]bool

type plurkSubManager struct {
	telepathy.ServicePlugin
	sync.Mutex
	session *telepathy.Session
	subList *sync.Map
	context context.Context
	logger  *logrus.Entry
}

func init() {
	telepathy.RegisterService(funcKey, ctor)
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	manager := &plurkSubManager{
		session: param.Session,
		subList: &sync.Map{},
		logger:  param.Logger,
	}
	manager.session.WebServer.RegisterWebhook(funcKey, manager.webhook)
	return manager, nil
}

func (m *plurkSubManager) Start(context context.Context) {
	m.context = context
	<-m.loadFromDB()
}

func (m *plurkSubManager) ID() string {
	return funcKey
}

func (m *plurkSubManager) webhook(response http.ResponseWriter, req *http.Request) {
	// Validate request
	if req.Method != "POST" {
		response.WriteHeader(405)
		return
	} else if req.Header.Get("secret") != os.Getenv("PLURK_SECRET") {
		response.WriteHeader(401)
		return
	}

	// Parse new plurk post notification
	if req.ParseForm() != nil {
		response.WriteHeader(500)
		return
	}

	response.WriteHeader(200)

	plurkUser := req.PostFormValue("raw__author__name")
	plurkContent := req.PostFormValue("content")
	plurkLink := req.PostFormValue("link")
	plurkDate, timeErr := time.Parse(time.RFC3339, req.PostFormValue("pubDate"))
	plurkDateStr := ""

	// If we cannot find plurk user or plurk content, just ignore this request
	if plurkUser == "" || plurkContent == "" {
		return
	}

	// If some channels are subscribing this user, send the content to the channels
	load, ok := m.subList.Load(plurkUser)
	if !ok {
		return
	}

	// Perform timezone convert and stringify
	if timeErr == nil {
		if Location, locErr := time.LoadLocation(timeZone); locErr == nil {
			plurkDateStr = plurkDate.In(Location).Format("2006-01-02 15:04")
		}
	}

	// Start sending messages to channels
	targets := load.(channelList)
	msgBody := fmt.Sprintf(`[Plurk RSS Feed]
Author: %s
Date/Time: %s
%s
---
%s`, plurkUser, plurkDateStr, plurkContent, "https://www.plurk.com"+plurkLink)

	for channel := range targets {
		msg := &telepathy.OutboundMessage{
			TargetID: channel.ChannelID,
			Text:     msgBody,
		}
		msgr, _ := m.session.Message.Messenger(channel.MessengerID)
		msgr.Send(msg)
	}
}

func tableToBSON(table *sync.Map) *bson.Document {
	doc := bson.NewDocument(bson.EC.String("type", dbType))
	table.Range(func(key interface{}, value interface{}) bool {
		user, _ := key.(string)
		toList, _ := value.(channelList)
		bsonToList := bson.NewArray()
		for to := range toList {
			bsonToList.Append(bson.VC.String(to.JSON()))
		}
		doc.Append(bson.EC.Array(user, bsonToList))
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
			user := element.Key()
			toIter, _ := toList.Iterator()
			for {
				if !toIter.Next() {
					break
				}
				toCh := &telepathy.Channel{}
				jsonStr := toIter.Value().StringValue()
				json.Unmarshal([]byte(jsonStr), toCh)
				createSubNoLock(&user, toCh, table)
			}
		}
	}
	return table
}

func (m *plurkSubManager) writeToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			doc := tableToBSON(m.subList)
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"type": dbType}, doc,
				replaceopt.Upsert(true))
			if err != nil {
				m.logger.Error("error when write-back to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (m *plurkSubManager) loadFromDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			m.Lock()
			defer m.Unlock()
			result := collection.FindOne(ctx, map[string]string{"type": dbType})
			doc := bson.NewDocument()
			err := result.Decode(doc)
			if err != nil {
				m.logger.Error("error when load from DB: " + err.Error())
			} else {
				m.subList = bsonToTable(doc)
			}
			m.logger.Info("load from DB done")
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (m *plurkSubManager) createSub(user *string, channel *telepathy.Channel) bool {
	m.Lock()
	defer m.Unlock()
	return createSubNoLock(user, channel, m.subList)
}

func createSubNoLock(user *string, channel *telepathy.Channel, table *sync.Map) bool {
	load, loaded := table.LoadOrStore(*user, channelList{*channel: true})
	// If not loaded, new subscriber is stored
	if loaded {
		// Append new subscriber to the list
		subr, _ := load.(channelList)
		if subr[*channel] {
			return false // exists
		}
		subr[*channel] = true
	}
	return true
}

func (m *plurkSubManager) removeSub(user *string, channel *telepathy.Channel) bool {
	m.Lock()
	defer m.Unlock()
	load, ok := m.subList.Load(*user)
	if ok {
		subrList, _ := load.(channelList)
		if subrList[*channel] {
			delete(subrList, *channel)
			m.subList.Store(*user, subrList)
			return true
		}
	}
	return false
}

func (m *plurkSubManager) subscriptions(channel *telepathy.Channel) []string {
	subs := []string{}
	m.subList.Range(func(key, value interface{}) bool {
		subrList, _ := value.(channelList)
		if subrList[*channel] {
			user, _ := key.(string)
			subs = append(subs, user)
		}
		return true
	})
	return subs
}
