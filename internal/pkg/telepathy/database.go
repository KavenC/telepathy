package telepathy

import (
	"context"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"
)

// DatabaseRequest defines a request for database
// When a DatabaseRequest is handled, the Action function is called
// and the return value will be pushed to Return channel
// The Action function is guaranteed to be run atomically without other DatabaseRequest
type DatabaseRequest struct {
	Action func(context.Context, *mongo.Database) interface{}
	Return chan interface{}
}

type databaseHandler struct {
	dbName   string
	client   *mongo.Client
	database *mongo.Database
	reqQueue chan *DatabaseRequest
	logger   *logrus.Entry
}

func newDatabaseHandler(mongourl string, dbname string) (*databaseHandler, error) {
	logger := logrus.WithField("module", "database")
	handler := databaseHandler{dbName: dbname}
	var err error
	handler.client, err = mongo.NewClient(mongourl)
	if err != nil {
		logger.Error("fail to create mongo client")
		return nil, err
	}
	handler.reqQueue = make(chan *DatabaseRequest)
	handler.logger = logger
	logger.Info("created database handler")
	return &handler, nil
}

func (h *databaseHandler) start(ctx context.Context) {
	h.logger.Info("starting")
	timeCtx, cancel := context.WithTimeout(ctx, time.Minute)
	h.logger.Info("connecting to MongoDB")
	err := h.client.Connect(timeCtx)
	cancel()
	if err != nil {
		h.logger.Errorf("connect failed: %s", err.Error())
		return
	}
	h.logger.Info("using database name: " + h.dbName)
	h.database = h.client.Database(h.dbName)

	h.logger.Info("waiting for request")
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("terminated")
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := h.client.Disconnect(timeoutCtx)
			cancel()
			if err != nil {
				h.logger.Errorf("failed to disconnect: %s", err.Error())
			} else {
				h.logger.Info("disconnected")
			}
			return
		case request := <-h.reqQueue:
			ret := request.Action(ctx, h.database)
			request.Return <- ret
		}
	}
}

// PushRequest pushes a new database request
func (h *databaseHandler) PushRequest(req *DatabaseRequest) {
	h.reqQueue <- req
}
