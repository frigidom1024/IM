package test

import (
	"testing"
	"time"

	"gochat/internal/pkg/config"
	"gochat/internal/pkg/redis"
)

func TestRedisConnection(t *testing.T) {
	cfg := config.MustLoad("../config/config-dev.yaml")

	if err := redis.Init(&cfg.Redis); err != nil {
		t.Fatalf("init redis failed: %v", err)
	}
	defer redis.Close()

	// 清理测试数据
	redis.Del("test_key", "test_queue")

	// 测试SET/GET
	testKey := "test_key"
	testValue := map[string]string{"hello": "world"}

	if err := redis.Set(testKey, testValue, time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	var result map[string]string
	if err := redis.Get(testKey, &result); err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if result["hello"] != "world" {
		t.Errorf("expected world, got %s", result["hello"])
	}

	// 测试队列
	queueName := "test_queue"
	msg := map[string]string{"type": "test"}
	if err := redis.Push(queueName, msg); err != nil {
		t.Fatalf("push failed: %v", err)
	}

	size, err := redis.QueueSize(queueName)
	if err != nil {
		t.Fatalf("queue size failed: %v", err)
	}

	if size != 1 {
		t.Errorf("expected queue size 1, got %d", size)
	}

	t.Log("Redis connection test passed")
}
