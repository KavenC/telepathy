package telepathy

import (
	"context"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
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
	client   *redis.Client
	reqQueue chan *RedisRequest
	logger   *logrus.Entry
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

	// TODO: define queue length
	handle.reqQueue = make(chan *RedisRequest)

	handle.logger = logger

	logger.Info("created redis handler")
	return &handle, nil
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

	// Queue handling routine
	go func() {
		for {
			select {
			case <-ctx.Done():
				r.logger.Info("terminated")
				err := r.client.Close()
				if err != nil {
					r.logger.Errorf("failed to close connection: %s", err.Error())
				} else {
					r.logger.Info("connection closed")
				}
				return
			case request := <-r.reqQueue:
				ret := request.Action(r.client.WithContext(ctx))
				request.Return <- ret
			}
		}
	}()
}

// PushRequest pushes a new redis request
func (r *redisHandle) PushRequest(req *RedisRequest) {
	r.reqQueue <- req
}
