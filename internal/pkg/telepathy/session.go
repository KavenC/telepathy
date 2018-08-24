package telepathy

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var _ = func() bool {
	_, production := os.LookupEnv("TELEPATHY_PRODUCTION")
	if !production {
		logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
		logrus.Info("== Telepathy Starts in DEVELOPMENT Mode ==")
	} else {
		logrus.Info("== Telepathy Starts in PRODUCTION Mode ==")
	}
	return true
}()

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
	// Start redis
	go s.Redis.start(ctx)

	// Start database
	go s.DB.start(ctx)

	// Start messenger handlers
	for _, messenger := range s.Msgr.messengers {
		go messenger.Start(ctx)
	}

	// Webhook stype messengers are handled together with a http server
	logrus.Info("Start listening port: " + s.port)
	server := http.Server{Addr: ":" + s.port, Handler: serveMux()}
	go server.ListenAndServe()

	// Wait here until the session is Done
	<-ctx.Done()

	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	// Shutdown Http server
	server.Shutdown(timeout)
}
