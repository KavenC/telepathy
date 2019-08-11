// Package twitch provides twtitch.tv services for package telepathy
// Needed  Config
// - CLIENT_ID: The client ID of twitch api
package twitch

import (
	"net/url"
	"sync"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is the plugin id
const id = "twitch"

const twitchURL = "https://www.twitch.tv/"

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

	// webhookSubs: Webhook type -> UserID -> Subscriber channel
	webhookSubs  map[string]*table
	streamStatus sync.Map // UserID -> stream status

	// The client ID of twitch API
	ClientID string
	// HMAC secret for validating incoming notifications
	WebsubSecret string

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
	s.api = newTwitchAPI()
	s.api.clientID = s.ClientID
	s.api.websubSecret = s.WebsubSecret
	s.api.webhookURL = s.webhookURL
	s.api.logger = s.logger.WithField("module", "api")

	s.webhookSubs = make(map[string]*table)

	// - Supported Webhooks
	s.webhookSubs["streams"] = newTable()

	// - Load Table content from database

	s.logger.Info("started")
	// Wait for close
	<-s.cmdDone
	// Cancel all websub renewal routines
	s.api.renewCancelAll()
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
