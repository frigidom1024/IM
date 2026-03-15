package redis

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func Push(queueName string, message any) error {
	data, err := json.Marshal(message)
	if err != nil {

		return fmt.Errorf("marshal message failed: %w", err)

	}

	return Client.RPush(ctx, queueName, data).Err()
}

// Pop 从队列弹出消息（阻塞）
func Pop(queueName string, timeout time.Duration) (string, error) {
	result, err := Client.BRPop(ctx, timeout, queueName).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}

	// BRPop返回 [queueName, value]
	if len(result) < 2 {
		return "", nil
	}

	return result[1], nil
}

// QueueSize 获取队列大小
func QueueSize(queueName string) (int64, error) {
	return Client.LLen(ctx, queueName).Result()
}
