package telepathy

import (
	"github.com/sirupsen/logrus"
)

var resourceCtors = make(map[string]ResourceCtor)

// ResourceCtor is the callback function creating certain resource
type ResourceCtor func(*Session) (interface{}, error)

// ResourceExistsError indicates that registering a existing resource
type ResourceExistsError struct {
	name string
}

func (e ResourceExistsError) Error() string {
	return "Resource with name: " + e.name + " is already registered"
}

// RegisterResource register an arbitrary resource that will be alive with Teleathy session
// registered constructors will be called after database and redis are ready and before
// messengers are started
func RegisterResource(name string, value ResourceCtor) error {
	logger := logrus.WithFields(logrus.Fields{
		"module": "resource",
		"name":   name})
	if resourceCtors[name] != nil {
		logger.Error("resource exists")
		return ResourceExistsError{name}
	}

	resourceCtors[name] = value
	logger.Info("registered")

	return nil
}
