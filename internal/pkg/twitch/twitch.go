// Package twitch provides twtitch.tv services for package telepathy
// Needed  Config
// - CLIENT_ID: The client ID of twitch api
package twitch

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"gitlab.com/kavenc/telepathy/internal/pkg/randstr"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is the plugin id
const id = "twitch"

const twitchURL = "https://www.twitch.tv/"

type notification struct {
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

	webhookURL *url.URL

	api *twitchAPI

	subTopics     map[string]*table // topic -> user id -> [channels]
	verifyingSubs sync.Map

	// Notification handling routine
	notifQueue  chan *notification
	notifCtx    context.Context
	notifCancel context.CancelFunc
	notifDone   chan interface{}

	// Sub renew routine
	renewCtx       context.Context // context controls all websub renewal routines
	renewCancel    context.CancelFunc
	renewCancelMap sync.Map

	streamStatus sync.Map // UserID -> stream status

	// HMAC secret for validating incoming notifications
	websubSecret []byte

	// The client ID of twitch API
	ClientID string

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
	s.notifQueue = make(chan *notification, 10)
	s.notifCtx, s.notifCancel = context.WithCancel(context.Background())
	s.notifDone = make(chan interface{})

	s.renewCtx, s.renewCancel = context.WithCancel(context.Background())

	s.websubSecret = []byte(randstr.Generate(32))
	s.api = newTwitchAPI()
	s.api.clientID = s.ClientID
	s.api.websubSecret = string(s.websubSecret)
	s.api.webhookURL = s.webhookURL
	s.api.logger = s.logger.WithField("module", "api")

	s.subTopics = make(map[string]*table)
	// - Supported Topics
	s.subTopics["streams"] = newTable()

	// - TODO: Load Table content from database

	go s.notifHandler()

	s.logger.Info("started")
	// Wait for close
	<-s.cmdDone

	// TODO: write back db

	// Cancel all websub renewal routines
	s.renewCancel()

	// Terminate websub handling
	<-s.notifDone
	close(s.notifQueue)
	close(s.msgOut)

	// wait for writeback
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
