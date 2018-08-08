package telepathy

import (
	"context"
	"net/http"
	"os"
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
	Ctx          context.Context
	DatabaseType string
	Port         string
}

// Start starts a Telepathy session
// The function always returns an error when the seesion is terminated
func (s *Session) Start() error {
	if s.Ctx == nil {
		logrus.Panic("Session Ctx must not be nil")
	}

	// init cache
	err := initRedis()
	if err != nil {
		logrus.Panic("Redis init failed.")
	}

	// Initializes database handler
	err = initDatabase(s.DatabaseType)
	if err != nil {
		logrus.Panic("Database init failed.")
	}

	// Initializes messenger handlers
	for _, messenger := range messengerList {
		err := messenger.init()
		if err != nil {
			logger := logrus.WithField("messenger", messenger.name())
			logger.Error("Init fail.")
		}
	}

	ctx, cancel := context.WithCancel(s.Ctx)

	// Starts messenger handlers
	for _, messenger := range messengerList {
		go messenger.start(ctx)
	}
	// Webhook stype messengers are handled together with a http server
	server := http.Server{Addr: ":" + s.Port, Handler: getServeMux()}
	go server.ListenAndServe()

	// Wait here until the session is Done
	<-s.Ctx.Done()
	cancel()

	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	return server.Shutdown(timeout)

}
