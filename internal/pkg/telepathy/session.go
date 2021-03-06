package telepathy

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Session defines a Telepathy server session
type Session struct {
	ctx       context.Context
	db        *databaseHandler
	webServer *httpServer
	router    *router
	plugins   map[string]Plugin
	done      chan interface{}
	logger    *logrus.Entry
}

// SessionConfig defines the configurations of a Telepathy session
type SessionConfig struct {
	Port         string // Port Number for Webhook handling server
	RootURL      string // URL to telepathy server
	MongoURL     string // URL to the MongoDB Server
	DatabaseName string // MongoDB database name
}

// NewSession creates a new Telepathy session
func NewSession(config SessionConfig, plugins []Plugin) (*Session, error) {
	session := Session{
		plugins: make(map[string]Plugin),
		logger:  logrus.WithField("module", "session"),
	}

	session.logger.Info("initializing")

	var err error
	// initialize backend services
	// Init webserver
	session.webServer, err = newWebServer(config.RootURL, config.Port)
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

	// install plugins
	for _, p := range plugins {
		if _, ok := session.plugins[p.ID()]; ok {
			session.logger.Panicf("duplicated plugin id: %s", p.ID())
		}
		session.plugins[p.ID()] = p
	}

	// install internal plugins
	chPlugin := &channelService{}
	if _, ok := session.plugins[chPlugin.ID()]; ok {
		return nil, fmt.Errorf("Invalid Plugin ID: %s", chPlugin.ID())
	}
	session.plugins[chPlugin.ID()] = chPlugin

	session.initPlugin()

	return &session, nil
}

func (s *Session) initPlugin() {
	// For each plugin go through all implemented interfaces and
	// fuse them with framework modules
	logger := s.logger.WithField("phase", "init-plugin")
	for id, p := range s.plugins {
		logger.Infof("init plugin: %s", id)
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
			err := s.router.cmd.attachCommandInterface(pcmd.Command(s.router.cmd.done))
			if err != nil {
				logger.WithField("plugin", p.ID()).Panicf(err.Error())
			}
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
	}
}

// Start starts a Telepathy session
func (s *Session) Start(ctx context.Context) {
	s.done = make(chan interface{})

	wgBackend := sync.WaitGroup{}
	// Start database
	go func() {
		wgBackend.Add(1)
		err := s.db.start(ctx)
		if err != nil {
			s.db.logger.Errorf(err.Error())
		}
		wgBackend.Done()
	}()

	// Start plugins
	wgPlugin := sync.WaitGroup{}
	startPlugin := func(f func()) {
		wgPlugin.Add(1)
		f()
		wgPlugin.Done()
	}
	for _, plugin := range s.plugins {
		go startPlugin(plugin.Start)
	}

	// Start router
	go func() {
		wgBackend.Add(1)
		s.router.start(ctx, 5*time.Second, 5*time.Second)
		wgBackend.Done()
	}()

	// Start Webhook handling server
	s.logger.Info("starting web server")
	s.webServer.finalize()
	go s.webServer.ListenAndServe()

	// Wait here until we received termination signal
	<-s.done
	s.logger.Info("terminating")

	// Termination process
	// Shutdown Http server
	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.webServer.Shutdown(timeout)
	stop()
	if err != nil {
		s.logger.Errorf("failed to shutdown httpserver: %s", err.Error())
		return
	}
	s.logger.Info("httpserver shutdown")

	// Terminate plugins
	for _, plugin := range s.plugins {
		plugin.Stop()
	}
	wgPlugin.Wait()
	s.logger.Info("all plugins terminated")

	// Wait for backend service
	wgBackend.Wait()
	s.logger.Info("all backend services terminated")

	s.logger.Info("session closed")
}

// Stop triggers termination of telepathy session
func (s *Session) Stop() {
	close(s.done)
}
