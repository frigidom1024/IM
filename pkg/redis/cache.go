package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Set 设置缓存
func Set(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return Client.Set(context.Background(), key, data, expiration).Err()
}

// Get 获取缓存
func Get(key string, dest interface{}) error {
	data, err := Client.Get(context.Background(), key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil // key不存在
		}
		return err
	}

	return json.Unmarshal(data, dest)
}

// Del 删除缓存
func Del(keys ...string) error {
	return Client.Del(context.Background(), keys...).Err()
}

// Exists 检查key是否存在
func Exists(keys ...string) (int64, error) {
	return Client.Exists(context.Background(), keys...).Result()
}

// Expire 设置过期时间
func Expire(key string, expiration time.Duration) error {
	return Client.Expire(context.Background(), key, expiration).Err()
}
