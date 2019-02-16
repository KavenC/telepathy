package plurkrss

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const timeZone = "Asia/Taipei"
const dbType = "plurksubtable"

type plurkSubManager struct {
	telepathy.ServicePlugin
	session *telepathy.Session
	subList *telepathy.ChannelListMap
	context context.Context
	logger  *logrus.Entry
}

func init() {
	telepathy.RegisterService(funcKey, ctor)
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	manager := &plurkSubManager{
		session: param.Session,
		subList: telepathy.NewChannelListMap(),
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
	targets, ok := m.subList.GetList(plurkUser)
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

func (m *plurkSubManager) writeToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	m.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			result, err := m.subList.StoreToDB(ctx, collection, dbType)
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
			err := m.subList.LoadFromDB(ctx, collection, dbType, reflect.TypeOf(""))
			if err != nil {
				m.logger.Error("error when load from DB: " + err.Error())
			}
			m.logger.Info("load from DB done")
			return err
		},
		Return: retCh,
	})
	return retCh
}

func (m *plurkSubManager) subscriptions(channel *telepathy.Channel) []string {
	subs := []string{}
	m.subList.Range(func(key interface{}, subrList telepathy.ChannelList) bool {
		if subrList[*channel] {
			user, _ := key.(string)
			subs = append(subs, user)
		}
		return true
	})
	return subs
}
