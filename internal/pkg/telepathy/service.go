package telepathy

import (
	"github.com/sirupsen/logrus"
)

// ServiceCtorParam is the data structured pass to ServiceCtor
type ServiceCtorParam struct {
	Session *Session
	Config  PluginConfig
	Logger  *logrus.Entry
}

// ServiceCtor is constructor function used to create a Service instance
type ServiceCtor func(*ServiceCtorParam) (Service, error)

// Service defines the functions that a Service plugin needs to implement
type Service interface {
	plugin
}
