package telepathy

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Session defines a Telepathy server session
type Session struct {
	ctx       context.Context
	Redis     *redisHandle
	DB        *databaseHandler
	WebServer httpServer
	Command   *cmdManager

}

// SessionConfig defines the configurations of a Telepathy session
type SessionConfig struct {
	// Infrastructure configs
	Port         string // Port Number for Webhook handling server
	RootURL      string // URL to telepathy server
	RedisURL     string // URL to the Redis server
	MongoURL     string // URL to the MongoDB Server
	DatabaseName string // MongoDB database name
	// Plugin Config Table
	Plugins map[string]plugin
}

// NewSession creates a new Telepathy session
func NewSession(config SessionConfig) (*Session, error) {
	session := Session{
		WebServer: httpServer{},
	}
	var err error
	session.WebServer.uRL, err = url.Parse(config.RootURL)
	if err != nil {
		return nil, err
	}

	// Init redis
	session.Redis, err = newRedisHandle(config.RedisURL)
	if err != nil {
		return nil, err
	}

	// Init database
	session.DB, err = newDatabaseHandler(config.MongoURL, config.DatabaseName)
	if err != nil {
		return nil, err
	}

	// Init command manager
	session.Command = newCmdManager(&session)

	// Init messenger
	session.Messenger = newMessengerManager(&session, config.MessengerConfigTable)

	// Init service
	// Internal service
	RegisterService(channelServiceID, newChannelService)
	session.Service = newServiceManager(&session, config.ServiceConfigTable)

	// Init httpServer
	err = session.WebServer.init(config.Port)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// Start starts a Telepathy session
// The function always returns an error when the seesion is terminated
func (s *Session) Start(ctx context.Context) {
	logrus.Info("session start")

	var wg sync.WaitGroup
	// Start backend services
	logrus.Info("starting backend services")
	wg.Add(2)
	// Start redis
	go func() {
		s.Redis.start(ctx)
		wg.Done()
	}()

	// Start database
	go func() {
		s.DB.start(ctx)
		wg.Done()
	}()
	wg.Wait()

	// Start messenger handlers
	logrus.Info("starting messengers")
	s.Messenger.startMessengers()
	go s.Messenger.start()

	// Start services
	logrus.Info("starting services")
	s.Service.startServices()
	go s.Service.start()

	//Start Webhook handling server
	logrus.WithField("module", "session").Info("starting web server")
	s.WebServer.finalize()
	go s.WebServer.ListenAndServe()

	// Wait here until the session is Done
	<-ctx.Done()
	logrus.WithField("module", "session").Info("stopping")

	// Shutdown Http server
	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.WebServer.Shutdown(timeout)
	stop()
	if err != nil {
		logrus.Errorf("failed to shutdown httpserver: %s", err.Error())
	} else {
		logrus.Info("httpserver shutdown")
	}
	logrus.Info("session closed")
}
