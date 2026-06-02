package connect

import (
	"context"
	"gochat/pkg/logger"

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
	// 这里可以添加用户下线时的处理逻辑
	// 比如：更新在线状态、清理缓存等
	return nil
}
