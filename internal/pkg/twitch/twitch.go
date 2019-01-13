// Package twitch provides twtitch.tv services for package telepathy
// Needed  Config
// - CLIENT_ID: The client ID of twitch api
package twitch

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is the plugin id
const ID = "twitch"

type twitchService struct {
	telepathy.ServicePlugin
	session *telepathy.Session
	api     *twitchAPI
	ctx     context.Context
	logger  *logrus.Entry
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
		session: param.Session,
		api:     newTwitchAPI(),
		logger:  param.Logger,
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
	service.session.WebServer.RegisterWebhook("twitch", service.webhook)

	return service, nil
}

func (s *twitchService) Start(ctx context.Context) {
	s.ctx = ctx
}

func (s *twitchService) ID() string {
	return ID
}

func (s *twitchService) webhook(response http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		response.WriteHeader(405)
		return
	}

	respChan := make(chan int, 1)
	go s.handleWebhookReq(s.ctx, req, respChan)

	select {
	case resp := <-respChan:
		response.WriteHeader(resp)
		return
	case <-s.ctx.Done():
		response.WriteHeader(503)
		return
	}
}

func (s *twitchService) handleWebhookReq(ctx context.Context, req *http.Request, resp chan int) {
	// TODO: handle unsubscribed callback, resp = 410

	// TODO: Auth

	// Accepted
	resp <- 200

}
