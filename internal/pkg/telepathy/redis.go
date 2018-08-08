package telepathy

import (
	"os"

	"github.com/go-redis/redis"
)

var client *redis.Client

func initRedis() error {
	options, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		return err
	}

	client := redis.NewClient(options)
	_, err = client.Ping().Result()
	if err != nil {
		return err
	}

	// Flush all
	err = client.FlushAll().Err()

	return err
}

func getRedis() *redis.Client {
	return client
}
