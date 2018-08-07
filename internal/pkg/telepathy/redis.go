package telepathy

import (
	"os"
	"sync"

	"github.com/go-redis/redis"
)

var redisClient *redis.Client
var redisLock = &sync.Mutex{}

func initRedis() error {
	options, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		return err
	}

	redisClient = redis.NewClient(options)
	_, err = redisClient.Ping().Result()
	if err != nil {
		return err
	}

	// Flush all
	err = redisClient.FlushAll().Err()

	return err
}

func getRedis() *redis.Client {
	redisLock.Lock()
	return redisClient
}

func putRedis() {
	redisLock.Unlock()
}
