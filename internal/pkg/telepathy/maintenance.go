package telepathy

import (
	"errors"
	"net/http"
)

var database Database

// Start starts a telepathy server
func Start(databaseType string, port string) error {
	getter := databaseList[databaseType]
	if getter == nil {
		return errors.New("Database type not found: " + databaseType)
	}
	database = getter()

	for _, messenger := range messengerList {
		messenger.start()
	}

	server := http.Server{Addr: ":" + port, Handler: mux}
	return server.ListenAndServe()
}
