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
	redis     *redisHandle
	db        *databaseHandler
	webServer httpServer
	router    *router
	plugins   map[string]Plugin
	logger    *logrus.Entry
}

// SessionConfig defines the configurations of a Telepathy session
type SessionConfig struct {
	Port         string // Port Number for Webhook handling server
	RootURL      string // URL to telepathy server
	RedisURL     string // URL to the Redis server
	MongoURL     string // URL to the MongoDB Server
	DatabaseName string // MongoDB database name
}

// NewSession creates a new Telepathy session
func NewSession(config SessionConfig, plugins map[string]Plugin) (*Session, error) {
	session := Session{
		webServer: httpServer{},
		plugins:   plugins,
		logger:    logrus.WithField("module", "session"),
	}

	session.plugins["telepathy.channel"] = &channelService{}

	var err error
	session.webServer.uRL, err = url.Parse(config.RootURL)
	if err != nil {
		return nil, err
	}

	// Init redis
	session.redis, err = newRedisHandle(config.RedisURL)
	if err != nil {
		return nil, err
	}

	// Init database
	session.db, err = newDatabaseHandler(config.MongoURL, config.DatabaseName)
	if err != nil {
		return nil, err
	}

	// Init Router
	session.router = newRouter()

	// Init httpServer
	err = session.webServer.init(config.Port)
	if err != nil {
		return nil, err
	}

	// Init plugins
	session.initPlugin()

	return &session, nil
}

func (s *Session) initPlugin() {
	// For each plugin go through all implemented interfaces and
	// fuse them with framework modules
	logger := s.logger.WithField("phase", "init-plugin")
	for id, p := range s.plugins {
		// See plugin.go for interface definitions
		// first, we check if the id matches
		if id != p.ID() {
			logger.Panicf("plugin id mismatch, map id: %s plugin id: %s", id, p.ID())
		}
		p.SetLogger(logrus.WithField("plugin", id))

		// Go through all interface implementations
		if pmsg, ok := p.(PluginMessenger); ok {
			s.router.attachReceiver(id, pmsg.InMsgChannel())
			pmsg.AttachOutMsgChannel(s.router.attachTransmitter(id))
		}

		if pcmd, ok := p.(PluginCommandHandler); ok {
			s.router.cmd.attachCommandInterface(pcmd.Command())
		}

		if pwebh, ok := p.(PluginWebhookHandler); ok {
			urlMap := make(map[string]*url.URL)
			for key, handle := range pwebh.Webhook() {
				url, err := s.webServer.registerWebhook(key, handle)
				if err != nil {
					logger.Panicf(err.Error())
				}
				urlMap[key] = url
			}
			pwebh.SetWebhookURL(urlMap)
		}

		if pcon, ok := p.(PluginMsgConsumer); ok {
			pcon.AttachInMsgChannel(s.router.attachConsumer(id))
		}

		if ppro, ok := p.(PluginMsgProducer); ok {
			s.router.attachProducer(id, ppro.OutMsgChannel())
		}

		if pdb, ok := p.(PluginDatabaseUser); ok {
			s.db.attachRequester(id, pdb.DBRequestChannel())
		}

		if pred, ok := p.(PluginRedisUser); ok {
			s.redis.attachRequester(id, pred.RedisRequestChannel())
		}
	}
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
		s.redis.start(ctx)
		wg.Done()
	}()

	// Start database
	go func() {
		s.db.start(ctx)
		wg.Done()
	}()
	wg.Wait()

	//Start Webhook handling server
	logrus.WithField("module", "session").Info("starting web server")
	s.webServer.finalize()
	go s.webServer.ListenAndServe()

	// Wait here until the session is Done
	<-ctx.Done()
	logrus.WithField("module", "session").Info("stopping")

	// Shutdown Http server
	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.webServer.Shutdown(timeout)
	stop()
	if err != nil {
		logrus.Errorf("failed to shutdown httpserver: %s", err.Error())
	} else {
		logrus.Info("httpserver shutdown")
	}
	logrus.Info("session closed")
}
