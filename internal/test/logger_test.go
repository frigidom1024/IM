package test

import (
	"testing"

	"go.uber.org/zap"
	"gochat/internal/pkg/logger"
)

func TestLogger(t *testing.T) {
	if err := logger.Init("debug", "stdout"); err != nil {
		t.Fatalf("init logger failed: %v", err)
	}
	defer logger.Sync()

	logger.Debug("This is a debug message", zap.String("key", "value"))
	logger.Info("This is an info message", zap.Int("count", 100))
	logger.Warn("This is a warning message", zap.Error(nil))
	logger.Error("This is an error message", zap.String("error", "test error"))

	t.Log("Logger test passed")
}
