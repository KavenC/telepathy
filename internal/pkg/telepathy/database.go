package telepathy

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"
)

const (
	dBReqLen  = 10
	dBTimeout = time.Minute
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
	dbName       string
	client       *mongo.Client
	database     *mongo.Database
	reqQueue     chan DatabaseRequest
	requesterMap map[string]<-chan DatabaseRequest
	logger       *logrus.Entry
}

func newDatabaseHandler(mongourl string, dbname string) (*databaseHandler, error) {
	logger := logrus.WithField("module", "database")
	handler := databaseHandler{dbName: dbname}
	var err error
	handler.client, err = mongo.NewClient(mongourl)
	if err != nil {
		logger.Error("failed to create mongo client")
		return nil, err
	}
	handler.reqQueue = make(chan DatabaseRequest, dBReqLen)
	handler.logger = logger
	handler.requesterMap = make(map[string]<-chan DatabaseRequest)

	return &handler, nil
}

func (h *databaseHandler) attachRequester(id string, ch <-chan DatabaseRequest) {
	if _, ok := h.requesterMap[id]; ok {
		h.logger.Panicf("requester exists: %s", id)
	}
	h.requesterMap[id] = ch
}

func (h *databaseHandler) worker(ctx context.Context) {
	wg := sync.WaitGroup{}
	wg.Add(len(h.requesterMap))
	for _, reqCh := range h.requesterMap {
		go func(ch <-chan DatabaseRequest) {
			for req := range ch {
				h.reqQueue <- req
			}
			wg.Done()
		}(reqCh)
	}

	go func() {
		wg.Wait()
		close(h.reqQueue)
	}()

	for request := range h.reqQueue {
		timeout, cancel := context.WithTimeout(ctx, dBTimeout)
		done := make(chan interface{})
		go func() {
			ret := request.Action(timeout, h.database)
			request.Return <- ret
			close(done)
		}()

		select {
		case <-timeout.Done():
			h.logger.Warnf("request timeout/cancelled")
		case <-done:
		}
		cancel()
	}
}

func (h *databaseHandler) start(ctx context.Context) error {
	timeCtx, cancel := context.WithTimeout(ctx, time.Minute)
	err := h.client.Connect(timeCtx)
	cancel()
	if err != nil {
		return err
	}

	h.database = h.client.Database(h.dbName)
	h.logger.Infof("started. Database name: %s", h.dbName)
	h.worker(ctx)

	timeCtx, cancel = context.WithTimeout(ctx, time.Minute)
	err = h.client.Disconnect(timeCtx)
	cancel()
	if err != nil {
		return err
	}
	h.logger.Info("terminated")
	return nil
}
