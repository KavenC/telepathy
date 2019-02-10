// Package twitch provides twtitch.tv services for package telepathy
// Needed  Config
// - CLIENT_ID: The client ID of twitch api
package twitch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is the plugin id
const ID = "twitch"

const twitchURL = "https://www.twitch.tv/"

type twitchService struct {
	telepathy.ServicePlugin
	session    *telepathy.Session
	api        *twitchAPI
	webhookURL *url.URL
	// webhookSubs: Webhook type -> UserID -> Subsriber channel
	webhookSubs  map[string]map[string]map[telepathy.Channel]bool
	streamStatus map[string]bool // UserID -> stream status

	ctx    context.Context
	logger *logrus.Entry
}

func init() {
	telepathy.RegisterService(ID, ctor)
}

func newTwitchAPI() *twitchAPI {
	api := &twitchAPI{httpTransport: &http.Transport{}}
	api.httpClient = &http.Client{Transport: api.httpTransport}
	return api
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	// Construct service
	service := &twitchService{
		session:      param.Session,
		api:          newTwitchAPI(),
		logger:       param.Logger,
		webhookSubs:  make(map[string]map[string]map[telepathy.Channel]bool),
		streamStatus: make(map[string]bool),
	}
	service.api.logger = service.logger.WithField("phase", "api")

	clientID, ok := param.Config["CLIENT_ID"]
	if !ok {
		return nil, errors.New("CLIENT_ID is not set")
	}

	service.api.clientID, ok = clientID.(string)
	if !ok {
		return nil, fmt.Errorf("Invalid CLIENT_ID type: %T", clientID)
	}

	// Register Webhook
	var err error
	service.webhookURL, err = service.session.WebServer.RegisterWebhook("twitch", service.webhook)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func (s *twitchService) Start(ctx context.Context) {
	s.ctx = ctx
	<-s.streamChangeLoadFromDB()
}

func (s *twitchService) ID() string {
	return ID
}
