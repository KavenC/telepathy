package telepathy

import (
	"context"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"
)

var logger = logrus.WithField("module", "database")

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
	handler := databaseHandler{dbName: dbname}
	var err error
	handler.client, err = mongo.NewClient(mongourl)
	if err != nil {
		return nil, err
	}

	handler.logger = logrus.WithField("module", "database")

	return &handler, nil
}

func (h *databaseHandler) start(ctx context.Context) error {
	h.logger.Info("started")
	timeCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	h.logger.Info("connecting to MongoDB server")
	err := h.client.Connect(timeCtx)
	if err != nil {
		return err
	}
	h.database = h.client.Database(h.dbName)

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("context done. existing")
			return nil
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
