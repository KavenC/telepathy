package telepathy

import (
	"github.com/sirupsen/logrus"
)

// ServiceCtorParam is the data structured pass to ServiceCtor
type ServiceCtorParam struct {
	Session *Session
	Logger  *logrus.Entry
}

// ServiceCtor is constructor function used to create a Service instance
type ServiceCtor func(*ServiceCtorParam) (Service, error)

// Service implements backend logics behind Messenger handlers
type Service interface {
	plugin
}

// ServicePlugin has to be inhereted for service plugins
type ServicePlugin struct{}

// GlobalService hides functions of Service for external usage
type GlobalService interface{}

var serviceCtors map[string]ServiceCtor

type serviceManager struct {
	session  *Session
	services map[string]Service
	logger   *logrus.Entry
}

// ServiceExistsError indicates registering Messenger with a name that already exists in the list
type ServiceExistsError struct {
	Name string
}

// ServiceInvalidError indicates requesting a non-registered Messenger name
type ServiceInvalidError struct {
	Name string
}

func (e ServiceExistsError) Error() string {
	return "Service: " + e.Name + " has already been registered"
}

func (e ServiceInvalidError) Error() string {
	return "Service: " + e.Name + " not found"
}

// RegisterService registers a Service
func RegisterService(ID string, ctor ServiceCtor) error {
	logger := logrus.WithField("module", "serviceManager").WithField("service", ID)
	if serviceCtors == nil {
		serviceCtors = make(map[string]ServiceCtor)
	}

	if serviceCtors[ID] != nil {
		logger.Errorf("already registered: %s", ID)
		return ServiceExistsError{Name: ID}
	}

	serviceCtors[ID] = ctor
	logger.Infof("registered service: '%s'", ID)

	return nil
}

func newServiceManager(session *Session) *serviceManager {
	manager := serviceManager{
		session:  session,
		services: make(map[string]Service),
		logger:   logrus.WithField("module", "serviceManager"),
	}

	paramBase := ServiceCtorParam{
		Session: session,
	}

	for ID, ctor := range serviceCtors {
		param := paramBase
		param.Logger = logrus.WithField("service", ID)
		service, err := ctor(&param)
		if err != nil {
			manager.logger.WithField("service", ID).Errorf("failed to construct: %s", err.Error())
			continue
		}

		if ID != service.ID() {
			manager.logger.WithField("service", ID).Errorf("regisered ID: %s but created as ID: %s. not constructed.",
				ID, service.ID())
			continue
		}

		manager.services[ID] = service

		if cmd := service.CommandInterface(); cmd != nil {
			manager.session.Command.RegisterCommand(cmd)
		}

		manager.logger.WithField("service", ID).Infof("created service : %s", ID)
	}

	return &manager
}

// Service gets a constructed Service with ID
func (m *serviceManager) Service(ID string) (GlobalService, error) {
	service := m.services[ID]
	if service == nil {
		return nil, ServiceInvalidError{Name: ID}
	}
	return service, nil
}
