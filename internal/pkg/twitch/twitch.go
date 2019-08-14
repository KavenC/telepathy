// Package twitch provides twtitch.tv services for package telepathy
// Needed  Config
// - CLIENT_ID: The client ID of twitch api
package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"gitlab.com/kavenc/telepathy/internal/pkg/randstr"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is the plugin id
const id = "twitch"

const twitchURL = "https://www.twitch.tv/"

type webusubNotification struct {
	request *http.Request
	body    []byte
	status  chan int
}

// Service defines telepathy plugin
type Service struct {
	telepathy.Plugin
	telepathy.PluginCommandHandler
	telepathy.PluginWebhookHandler
	telepathy.PluginMsgProducer
	telepathy.PluginDatabaseUser

	cmdDone <-chan interface{}
	msgOut  chan telepathy.OutboundMessage
	dbReq   chan telepathy.DatabaseRequest

	api        *twitchAPI
	webhookURL *url.URL

	webhookSubs  map[string]*table // webhookSubs: Webhook type -> UserID -> Subscriber channel
	streamStatus sync.Map          // UserID -> stream status
	// websubNotificationQueue stores http requests from websub hub for the notfications
	websubNotificationQueue chan *webusubNotification
	websubHandlingCtx       context.Context
	webSubHandlingCancel    context.CancelFunc
	notificationHandlerDone chan interface{}

	// The client ID of twitch API
	ClientID string

	// HMAC secret for validating incoming notifications
	websubSecret []byte

	logger *logrus.Entry
}

// ID implements telepathy.Plugin interface
func (s *Service) ID() string {
	return "TWITCH"
}

// SetLogger implements telepathy.Plugin interface
func (s *Service) SetLogger(logger *logrus.Entry) {
	s.logger = logger
}

// Start implements telepathy.Plugin interface
func (s *Service) Start() {
	// Initialize
	s.websubSecret = []byte(randstr.Generate(32))
	s.websubNotificationQueue = make(chan *webusubNotification, 10)

	s.api = newTwitchAPI()
	s.api.clientID = s.ClientID
	s.api.websubSecret = string(s.websubSecret)
	s.api.webhookURL = s.webhookURL
	s.api.logger = s.logger.WithField("module", "api")

	s.webhookSubs = make(map[string]*table)

	// - Supported Webhooks
	s.webhookSubs["streams"] = newTable()

	// - Load Table content from database

	s.websubHandlingCtx, s.webSubHandlingCancel = context.WithCancel(context.Background())
	s.notificationHandlerDone = make(chan interface{})
	go s.notificationHandler()

	s.logger.Info("started")
	// Wait for close
	<-s.cmdDone
	// Cancel all websub renewal routines
	s.api.renewCancelAll()

	// Terminate websub handling
	s.webSubHandlingCancel()
	close(s.websubNotificationQueue)
	<-s.notificationHandlerDone

	close(s.msgOut)
	// TODO: write back db
	close(s.dbReq)

	s.logger.Info("terminated")
}

// Stop implements telepathy.Plugin interface
func (s *Service) Stop() {

}

// OutMsgChannel implements telepathy.PluginMsgProducer
func (s *Service) OutMsgChannel() <-chan telepathy.OutboundMessage {
	if s.msgOut == nil {
		s.msgOut = make(chan telepathy.OutboundMessage, 10)
	}
	return s.msgOut
}

// DBRequestChannel implements telepathy.PluginDatabaseUser
func (s *Service) DBRequestChannel() <-chan telepathy.DatabaseRequest {
	if s.dbReq == nil {
		s.dbReq = make(chan telepathy.DatabaseRequest, 1)
	}
	return s.dbReq
}

func (s *Service) handleWebsubNotification(req *http.Request, body []byte) <-chan int {
	status := make(chan int, 1)
	notification := webusubNotification{
		request: req,
		body:    body,
		status:  status,
	}
	go func() {
		select {
		case s.websubNotificationQueue <- &notification:
		case <-s.websubHandlingCtx.Done():
			// this request is dropped
			status <- 200
		}
	}()
	return status
}

// notificationHandler handles websub notification callbacks in centeralized manner
// we need to predefine the handlers so that it is possible to gracefully shutdown everything
func (s *Service) notificationHandler() {
	logger := s.logger.WithField("phase", "notificationHandler")
	for notification := range s.websubNotificationQueue {
		req := notification.request
		ret := notification.status
		body := notification.body
		// A "topic" query is appended as callback url when subscribing
		// Here we can use the "topic" query to identify the topic of this callback request
		topic := req.URL.Query()["topic"]
		if topic == nil {
			logger.Warnf("invalid callback with no topic query. URL: %s", req.URL.String())
			ret <- 400
			continue
		}

		// Take only the first mode parameters, ignore others
		switch topic[0] {
		case "streams":
			// stream changed
			ret <- s.streamChanged(req, body)
		default:
			// return sub as deleted for any unknown topics
			ret <- 410
		}
	}
	close(s.notificationHandlerDone)
}

// streamChanged handles webhook callbacks for stream changed event
func (s *Service) streamChanged(request *http.Request, body []byte) int {
	logger := s.logger.WithField("phase", "streamChanged")

	userID := request.URL.Query().Get("user_id")
	chList, ok := s.webhookSubs["streams"].getList(userID)
	if !ok {
		// no subscribers, reply 410 to terminate the subscription
		logger.Warnf("get callback but not subscribers, do unsub. user_id: %s", userID)
		return 410
	}

	// start a goroutine for the rest of process and return 200 for this request
	go func() {
		// Get user display name
		ctx, cancel := context.WithTimeout(s.websubHandlingCtx, reqTimeOut)
		userChan := s.api.userByID(ctx, userID)
		defer cancel()

		// Unmarshal callback body
		var streamList StreamList
		err := json.Unmarshal(body, &streamList)
		if err != nil {
			logger.Error("failed to decode request body")
			<-userChan
			return
		}

		var user *User
		select {
		case user, ok = <-userChan:
			if !ok {
				return
			} else if user == nil {
				logger.Errorf("user not found, id: %s", userID)
				return
			}
		case <-ctx.Done():
			logger.Warnf("userById timeout/cancelled")
			return
		}

		// Construct message
		var msg strings.Builder
		if len(streamList.Data) == 0 {
			fmt.Fprintf(&msg, "== Twitch Stream Offline==\n- Streamer: %s (%s)",
				user.DisplayName, user.Login)
			s.streamStatus.Store(userID, false)
		} else {
			stream := streamList.Data[0]
			value, loaded := s.streamStatus.LoadOrStore(userID, true)
			var status bool
			if !loaded {
				status = false
			} else {
				status, _ = value.(bool)
				s.streamStatus.Store(userID, true)
			}
			if status {
				fmt.Fprintf(&msg, "== Twitch Stream Update ==\n%s",
					s.api.printStream(s.websubHandlingCtx, stream, user.Login))
			} else {
				fmt.Fprintf(&msg, "== Twitch Stream Online ==\n%s",
					s.api.printStream(s.websubHandlingCtx, stream, user.Login))
			}
		}

		// Broadcast message
		for channel := range chList {
			outMsg := telepathy.OutboundMessage{
				ToChannel: channel,
				Text:      msg.String(),
			}
			select {
			case s.msgOut <- outMsg:
			case <-s.websubHandlingCtx.Done():
				logger.Warnf("message cancelled")
				break
			}
		}
	}()

	return 200
}
