// Package info provides Telepathy information to users
package info

import (
	"context"
	"fmt"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	id = "info"
)

var version = "dev"

type service struct {
	telepathy.ServicePlugin
}

func init() {
	telepathy.RegisterService(id, ctor)
}

func ctor(param *telepathy.ServiceCtorParam) (telepathy.Service, error) {
	svc := &service{}
	return svc, nil
}

func (s *service) Start(ctx context.Context) {}

func (s *service) ID() string {
	return id
}

func (s *service) CommandInterface() *argo.Action {
	cmd := &argo.Action{
		Trigger:    id,
		ShortDescr: "Telepathy infomation",
		Do:         show,
	}
	return cmd
}

func show(state *argo.State, extras ...interface{}) error {
	// TODO: make it universal rather than bind to heroku (SOURCE_VERSION)
	fmt.Fprintf(&state.OutputStr,
		`Telepathy: Universal Messenger Botting Platform
Version: alpha (%s)
Source: https://gitlab.com/kavenc/telepathy`, version)
	return nil
}
