package twitch

import (
	"context"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const serviceID = "twitch-sub"

type twitchSubService struct {
	telepathy.ServicePlugin
	session *telepathy.Session
	logger  *logrus.Entry
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	service := &twitchSubService{
		session: param.Session,
		logger:  param.Logger,
	}

	return service, nil
}

func (s *twitchSubService) Start(ctx context.Context) {

}

func (s *twitchSubService) ID() string {
	return serviceID
}
