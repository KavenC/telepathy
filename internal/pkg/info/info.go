// Package info provides Telepathy information to users
package info

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	id = "info"
)

var version = "dev"

// Service implements telepathy plugin interfaces
type Service struct {
	telepathy.Plugin
	telepathy.PluginCommandHandler
	logger *logrus.Entry
}

// ID implements telepathy.Plugin
func (s Service) ID() string {
	return "INFO"
}

// SetLogger implements telepathy.Plugin
func (s *Service) SetLogger(logger *logrus.Entry) {
	s.logger = logger
}

// Start implements telepathy.Plugin
func (s Service) Start() {
	s.logger.Info("started")
}

// Stop implements telepathy.Plugin
func (s Service) Stop() {
	s.logger.Info("terminated")
}

// Command implements telepathy.PluginCommandHandler
func (s Service) Command(_ <-chan interface{}) *argo.Action {
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
Source: https://gitlab.com/kavenc/telepathy`, version[:8])
	return nil
}
