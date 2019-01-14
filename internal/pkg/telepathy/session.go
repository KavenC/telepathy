package telepathy

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// Session defines a Telepathy server session
type Session struct {
	ctx       context.Context
	Redis     *redisHandle
	DB        *databaseHandler
	Message   *MessageManager
	Service   *serviceManager
	WebServer httpServer
	Command   *cmdManager
}

// SessionConfig defines the configurations of a Telepathy session
type SessionConfig struct {
	// Infrastructure configs
	Port         string // Port Number for Webhook handling server
	RedisURL     string // URL to the Redis server
	MongoURL     string // URL to the MongoDB Server
	DatabaseName string // MongoDB database name
	// Plugin Config Tables
	MessengerConfigTable map[string]PluginConfig
	ServiceConfigTable   map[string]PluginConfig
}

// NewSession creates a new Telepathy session
func NewSession(config SessionConfig) (*Session, error) {
	session := Session{
		WebServer: httpServer{},
	}
	var err error

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
	session.Message = newMessageManager(&session, config.MessengerConfigTable)

	// Init service
	// Internal service
	RegisterService(channelServiceID, newChannelService)
	session.Service = newServiceManager(&session, config.ServiceConfigTable)

	// Init httpServer
	session.WebServer.init(config.Port)

	return &session, nil
}

// Start starts a Telepathy session
// The function always returns an error when the seesion is terminated
func (s *Session) Start(ctx context.Context) {
	logrus.Info("session start")
	// Start redis
	go s.Redis.start(ctx)

	// Start database
	go s.DB.start(ctx)

	// Start messenger handlers
	for _, messenger := range s.Message.messengers {
		go messenger.Start(ctx)
	}

	// Start services
	for _, service := range s.Service.services {
		go service.Start(ctx)
	}

	//Start Webhook handling server
	logrus.WithField("module", "session").Info("starting web server")
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
