package telepathy

import (
	"errors"
	"net/http"

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

	// Starts messenger handlers
	for _, messenger := range messengerList {
		go messenger.start()
	}

	// Webhook stype messengers are handled together with a http server
	server := http.Server{Addr: ":" + port, Handler: mux}
	return server.ListenAndServe()
}
