package handler

import (
	"context"

	"gochat/internal/pkg/logger"

	"go.uber.org/zap"
)

// DefaultDisconnectHandler 默认断开处理器
type DefaultDisconnectHandler struct{}

// NewDefaultDisconnectHandler 创建默认断开处理器
func NewDefaultDisconnectHandler() *DefaultDisconnectHandler {
	return &DefaultDisconnectHandler{}
}

// OnDisconnect 用户断开时调用
func (h *DefaultDisconnectHandler) OnDisconnect(ctx context.Context, userID string) error {
	logger.Info("用户下线", zap.String("userID", userID))
	return nil
}
