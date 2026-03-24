package connect

import (
	"context"
	"gochat/pkg/logger"

	"go.uber.org/zap"
)

// DefaultConnectHandler 默认连接处理器
type DefaultConnectHandler struct {
	offlineNotifier *OfflineNotifier
	server          *WsConnServer
}

// NewDefaultConnectHandler 创建默认连接处理器
func NewDefaultConnectHandler(offlineNotifier *OfflineNotifier, server *WsConnServer) *DefaultConnectHandler {
	return &DefaultConnectHandler{
		offlineNotifier: offlineNotifier,
		server:          server,
	}
}

// OnConnect 用户连接时调用
func (h *DefaultConnectHandler) OnConnect(ctx context.Context, userID string) error {
	logger.Info("用户上线", zap.String("userID", userID))

	// 发送离线消息通知
	if h.offlineNotifier != nil {
		h.offlineNotifier.HandleUserConnect(userID, h.server)
	}

	return nil
}
