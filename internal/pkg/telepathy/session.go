package telepathy

import (
	"context"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

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
		logrus.Fatal("Session Ctx must not be nil")
	}

	// Initializes database handler
	setDatabase(s.DatabaseType)

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
