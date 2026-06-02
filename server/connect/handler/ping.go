package handler

import (
	"context"

	"gochat/connect"
	"gochat/pkg/logger"

	"go.uber.org/zap"
)

// PingHandler 心跳处理器（处理 CmdPing）
type PingHandler struct {
	connSvr connect.ConnServer
}

// NewPingHandler 创建心跳处理器
func NewPingHandler(connSvr connect.ConnServer) *PingHandler {
	return &PingHandler{connSvr: connSvr}
}

// Handle 处理心跳消息
func (h *PingHandler) Handle(ctx context.Context, userID string, msg *connect.WsMsg) error {
	logger.Debug("处理心跳", zap.String("userID", userID))

	pongMsg := connect.NewPongMsg()
	if err := h.connSvr.Send(userID, pongMsg); err != nil {
		logger.Error("发送pong失败", zap.String("userID", userID), zap.Error(err))
		return err
	}
	logger.Debug("发送pong成功", zap.String("userID", userID))
	return nil
}
