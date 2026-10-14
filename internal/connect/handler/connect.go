package handler

import (
	"context"

	"gochat/internal/pkg/logger"

	"go.uber.org/zap"
)

// DefaultConnectHandler 默认连接处理器
type DefaultConnectHandler struct{}

func NewDefaultConnectHandler() *DefaultConnectHandler { return &DefaultConnectHandler{} }

// OnConnect 用户连接时调用
func (h *DefaultConnectHandler) OnConnect(ctx context.Context, userID string) error {
	logger.Info("用户上线", zap.String("userID", userID))

	return nil
}
