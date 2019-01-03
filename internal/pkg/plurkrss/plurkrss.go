package plurkrss

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const timeZone = "Asia/Taipei"

type channelList map[telepathy.Channel]bool

type plurkSubManager struct {
	sync.Mutex
	session *telepathy.Session
	subList *sync.Map
}

func init() {
	telepathy.RegisterResource(funcKey, resourceCtor)
}

func manager(s *telepathy.Session) *plurkSubManager {
	load, ok := s.Resrc.Load(funcKey)
	if !ok {
		logger.Error("failed to load manager")
		// Return a dummy manager to keeps things going
		// But it wont work well for sure
		return &plurkSubManager{subList: &sync.Map{}}
	}
	ret, ok := load.(*plurkSubManager)
	if !ok {
		logger.Errorf("resource type error: %v", load)
		// Return a dummy manager to keeps things going
		// But it wont work well for sure
		return &plurkSubManager{subList: &sync.Map{}}
	}
	return ret
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
%s`, plurkUser, plurkDateStr, plurkContent)

	for channel := range targets {
		msg := &telepathy.OutboundMessage{
			TargetID: channel.ChannelID,
			Text:     msgBody,
		}
		msgr, _ := m.session.Msgr.Messenger(channel.MessengerID)
		msgr.Send(msg)
	}
}

func resourceCtor(s *telepathy.Session) (interface{}, error) {
	manager := &plurkSubManager{
		session: s,
		subList: &sync.Map{},
	}

	// TODO: load from DB

	telepathy.RegisterWebhook(funcKey, manager.webhook)

	return manager, nil
}

func (m *plurkSubManager) createSub(user *string, channel *telepathy.Channel) bool {
	m.Lock()
	defer m.Unlock()
	return m.createSubNoLock(user, channel)
}

func (m *plurkSubManager) createSubNoLock(user *string, channel *telepathy.Channel) bool {
	load, loaded := m.subList.LoadOrStore(*user, channelList{*channel: true})
	// If not loaded, new subscriber is stored
	if loaded {
		// Append new subscriber to the list
		subr, _ := load.(channelList)
		if subr[*channel] {
			return false // exists
		}
		subr[*channel] = true
	}
	logger.Infof("Subscription: %s -> %s", *user, *channel)
	return true
}
