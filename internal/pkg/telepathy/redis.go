package telepathy

import (
	"context"
	"sync"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
)

const (
	redisReqLen = 10
)

// RedisRequest defines a request to redis handler
// A RedisRequest can be pushed to redis handler's waiting queue
// Once the a RedisRequest is handled, the action function is called
// and the returned result is pushed to Return channel
type RedisRequest struct {
	Action func(*redis.Client) interface{}
	Return chan interface{}
}

type redisHandle struct {
	client       *redis.Client
	reqQueue     chan RedisRequest
	requesterMap map[string]<-chan RedisRequest
	logger       *logrus.Entry
}

func newRedisHandle(redisurl string) (*redisHandle, error) {
	logger := logrus.WithField("module", "redis")
	handle := redisHandle{}
	options, err := redis.ParseURL(redisurl)
	if err != nil {
		logger.Error("failed to parse reids url")
		return nil, err
	}

	handle.client = redis.NewClient(options)
	handle.reqQueue = make(chan RedisRequest, redisReqLen)
	handle.logger = logger

	logger.Info("created redis handler")
	return &handle, nil
}

func (r *redisHandle) attachRequester(id string, ch <-chan RedisRequest) {
	if _, ok := r.requesterMap[id]; ok {
		r.logger.Panicf("requester exists: %s", id)
	}
	r.requesterMap[id] = ch
}

func (r *redisHandle) worker(ctx context.Context) {
	wg := sync.WaitGroup{}
	wg.Add(len(r.requesterMap))
	for _, reqCh := range r.requesterMap {
		go func(ch <-chan RedisRequest) {
			for req := range ch {
				r.reqQueue <- req
			}
			wg.Done()
		}(reqCh)
	}

	go func() {
		wg.Wait()
		close(r.reqQueue)
	}()

	for request := range r.reqQueue {
		timeout, cancel := context.WithTimeout(ctx, dBTimeout)
		done := make(chan interface{})
		go func() {
			ret := request.Action(r.client.WithContext(timeout))
			request.Return <- ret
			close(done)
		}()
		select {
		case <-timeout.Done():
			r.logger.Warnf("request cancelled due to timeout/termination")
		case <-done:
		}
		cancel()
	}
}

func (r *redisHandle) start(ctx context.Context) {
	if err := r.client.Ping().Err(); err != nil {
		r.logger.Errorf("ping failed: %s", err.Error())
		return
	}

	if err := r.client.FlushAll().Err(); err != nil {
		r.logger.Errorf("failed to flush all:: %s", err.Error())
	}

	r.logger.Info("conneted to Redis.")

	r.worker(ctx)

	r.logger.Info("terminated")
}
