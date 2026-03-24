package connect

import (
	"context"
	"gochat/pkg/logger"

	"go.uber.org/zap"
)

// MessageHandler 消息处理器接口
type MessageHandler interface {
	Handle(ctx context.Context, userID string, msg *WsMsg) error
}

// ConnectHandler 连接处理器接口
type ConnectHandler interface {
	OnConnect(ctx context.Context, userID string) error
}

// DisconnectHandler 断开处理器接口
type DisconnectHandler interface {
	OnDisconnect(ctx context.Context, userID string) error
}

// HandlerRegistry 处理器注册表（支持注册多个处理器）
type HandlerRegistry struct {
	messageHandlers     []MessageHandler
	connectHandlers     []ConnectHandler
	disconnectHandlers  []DisconnectHandler
}

// NewHandlerRegistry 创建处理器注册表
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		messageHandlers:     make([]MessageHandler, 0),
		connectHandlers:     make([]ConnectHandler, 0),
		disconnectHandlers:  make([]DisconnectHandler, 0),
	}
}

// RegisterMessageHandler 注册消息处理器（追加）
func (r *HandlerRegistry) RegisterMessageHandler(h MessageHandler) {
	r.messageHandlers = append(r.messageHandlers, h)
	logger.Info("注册消息处理器", zap.String("handler", "MessageHandler"), zap.Int("count", len(r.messageHandlers)))
}

// SetMessageHandler 设置消息处理器（清空并设置单个）
func (r *HandlerRegistry) SetMessageHandler(h MessageHandler) {
	r.messageHandlers = []MessageHandler{h}
	logger.Info("设置消息处理器", zap.String("handler", "MessageHandler"))
}

// RegisterConnectHandler 注册连接处理器（追加）
func (r *HandlerRegistry) RegisterConnectHandler(h ConnectHandler) {
	r.connectHandlers = append(r.connectHandlers, h)
	logger.Info("注册连接处理器", zap.String("handler", "ConnectHandler"), zap.Int("count", len(r.connectHandlers)))
}

// SetConnectHandler 设置连接处理器（清空并设置单个）
func (r *HandlerRegistry) SetConnectHandler(h ConnectHandler) {
	r.connectHandlers = []ConnectHandler{h}
	logger.Info("设置连接处理器", zap.String("handler", "ConnectHandler"))
}

// RegisterDisconnectHandler 注册断开处理器（追加）
func (r *HandlerRegistry) RegisterDisconnectHandler(h DisconnectHandler) {
	r.disconnectHandlers = append(r.disconnectHandlers, h)
	logger.Info("注册断开处理器", zap.String("handler", "DisconnectHandler"), zap.Int("count", len(r.disconnectHandlers)))
}

// SetDisconnectHandler 设置断开处理器（清空并设置单个）
func (r *HandlerRegistry) SetDisconnectHandler(h DisconnectHandler) {
	r.disconnectHandlers = []DisconnectHandler{h}
	logger.Info("设置断开处理器", zap.String("handler", "DisconnectHandler"))
}

// HandleMessage 处理消息（调用所有注册的消息处理器）
func (r *HandlerRegistry) HandleMessage(ctx context.Context, userID string, msg *WsMsg) {
	for _, handler := range r.messageHandlers {
		if err := handler.Handle(ctx, userID, msg); err != nil {
			logger.Warn("消息处理失败",
				zap.String("userID", userID),
				zap.String("cmd", msg.Cmd),
				zap.Error(err))
		}
	}
}

// HandleConnect 处理连接（调用所有注册的连接处理器）
func (r *HandlerRegistry) HandleConnect(ctx context.Context, userID string) {
	for _, handler := range r.connectHandlers {
		if err := handler.OnConnect(ctx, userID); err != nil {
			logger.Warn("连接处理失败",
				zap.String("userID", userID),
				zap.Error(err))
		}
	}
}

// HandleDisconnect 处理断开（调用所有注册的断开处理器）
func (r *HandlerRegistry) HandleDisconnect(ctx context.Context, userID string) {
	for _, handler := range r.disconnectHandlers {
		if err := handler.OnDisconnect(ctx, userID); err != nil {
			logger.Warn("断开处理失败",
				zap.String("userID", userID),
				zap.Error(err))
		}
	}
}
