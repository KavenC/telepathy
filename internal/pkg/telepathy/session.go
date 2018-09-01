package telepathy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Session defines a Telepathy server session
type Session struct {
	ctx   context.Context
	port  string
	Redis *redisHandle
	DB    *databaseHandler
	Msgr  *MessengerManager
	Resrc sync.Map
}

// SessionConfig defines the configurations of a Telepathy session
type SessionConfig struct {
	Port         string
	RedisURL     string
	MongoURL     string
	DatabaseName string
}

// NewSession creates a new Telepathy session
func NewSession(config SessionConfig) (*Session, error) {
	session := Session{port: config.Port}
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

	// Init messenger
	session.Msgr = newMessengerManager(&session)

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

	// Init resources
	for name, ctor := range resourceCtors {
		resrc, err := ctor(s)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module": "resource",
				"name":   name,
			}).Error("resource init failed: " + err.Error())
			continue
		}
		s.Resrc.Store(name, resrc)
	}

	// Start messenger handlers
	for _, messenger := range s.Msgr.messengers {
		go messenger.Start(ctx)
	}

	// Webhook stype messengers are handled together with a http server
	logrus.WithField("module", "session").Info("start listening port: " + s.port)
	mux := serveMux()

	// Add a simple response at root
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, "Telepathy Bot is Running")
	})

	server := http.Server{Addr: ":" + s.port, Handler: mux}
	go server.ListenAndServe()

	// Wait here until the session is Done
	<-ctx.Done()
	logrus.WithField("module", "session").Info("stopping")

	// Shutdown Http server
	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	err := server.Shutdown(timeout)
	stop()
	if err != nil {
		logrus.Errorf("failed to shutdown httpserver: %s", err.Error())
	} else {
		logrus.Info("httpserver shutdown")
	}
	logrus.Info("session closed")
}
