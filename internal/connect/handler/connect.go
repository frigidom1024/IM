package handler

import (
	"context"

	"gochat/internal/connect"
	"gochat/internal/pkg/logger"

	"go.uber.org/zap"
)

// DefaultConnectHandler 默认连接处理器
type DefaultConnectHandler struct {
	offlineNotifier connect.OfflineNotifier
	server          connect.ConnServer
}

// NewDefaultConnectHandler 创建默认连接处理器
func NewDefaultConnectHandler(offlineNotifier connect.OfflineNotifier, server connect.ConnServer) *DefaultConnectHandler {
	return &DefaultConnectHandler{
		offlineNotifier: offlineNotifier,
		server:          server,
	}
}

// OnConnect 用户连接时调用
func (h *DefaultConnectHandler) OnConnect(ctx context.Context, userID string) error {
	logger.Info("用户上线", zap.String("userID", userID))

	if h.offlineNotifier != nil {
		h.offlineNotifier.HandleUserConnect(userID, h.server)
	}

	return nil
}
