package telepathy

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

var database Database

// Start starts a telepathy server
func Start(databaseType string, port string) error {
	logger := logrus.WithFields(logrus.Fields{"DB": databaseType, "PORT": port})
	logger.Info("Telepathy Server in Starting")
	getter := databaseList[databaseType]
	if getter == nil {
		return errors.New("Database type not found: " + databaseType)
	}
	database = getter()

	// Initializes messenger handlers
	for _, messenger := range messengerList {
		err := messenger.init()
		if err != nil {
			logger := logrus.WithField("messenger", messenger.name())
			logger.Error("Init fail.")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Starts messenger handlers
	for _, messenger := range messengerList {
		go messenger.start(ctx)
	}
	// Webhook stype messengers are handled together with a http server
	server := http.Server{Addr: ":" + port, Handler: mux}
	go server.ListenAndServe()

	// Wait here until CTRL-C or other term signal is received.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	cancel()

	timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	return server.Shutdown(timeout)
}
