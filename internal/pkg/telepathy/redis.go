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
	handle := redisHandle{}
	options, err := redis.ParseURL(redisurl)
	if err != nil {
		return nil, err
	}

	handle.client = redis.NewClient(options)
	_, err = handle.client.Ping().Result()
	if err != nil {
		return nil, err
	}

	// Flush all
	err = handle.client.FlushAll().Err()
	if err != nil {
		return nil, err
	}

	// TODO: define queue length
	handle.reqQueue = make(chan *RedisRequest)

	handle.logger = logrus.WithField("module", "redis")

	return &handle, nil
}

func (r *redisHandle) start(ctx context.Context) {
	r.logger.Info("started")
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("context done. existing")
		case request := <-r.reqQueue:
			ret := request.Action(r.client.WithContext(ctx))
			request.Return <- ret
		}
	}
}

// PushRequest pushes a new redis request
func (r *redisHandle) PushRequest(req *RedisRequest) {
	r.reqQueue <- req
}
