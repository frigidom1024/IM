package redis

import (
	"context"
	"fmt"
	"gochat/internal/pkg/config"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client
var ctx = context.Background()

func Init(cfg *config.RedisConfig) error {

	Client = redis.NewClient(&redis.Options{

		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: 100,
	})

	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Connect redis failed:%w", err)
	}

	return nil
}

func Close() error {

	return Client.Close()

}

func GetClient() *redis.Client {

	return Client

}
