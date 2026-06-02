package connect

import (
	"context"
	"gochat/logic/model"
	"gochat/pkg/logger"

	"go.uber.org/zap"
)

// ==================== 接口定义 ====================

// MessageHandler 消息处理器
type MessageHandler interface {
	Handle(ctx context.Context, userID string, msg Message) error
}

// ConnectHandler 连接处理器
type ConnectHandler interface {
	OnConnect(ctx context.Context, userID string) error
}

// DisconnectHandler 断开处理器
type DisconnectHandler interface {
	OnDisconnect(ctx context.Context, userID string) error
}

// MessageService 消息服务接口
type MessageService interface {
	Send(msg *model.Message) error
	GetByID(id int64) (*model.Message, error)
	IncrUnread(userID, fromID, groupID int64) error
}

// GroupService 群组服务接口
type GroupService interface {
	GetMembers(groupID int64) ([]*model.GroupMember, error)
}

// ConnServer WebSocket 服务器接口（handler 需要的服务端能力）
type ConnServer interface {
	Send(userID string, msg Message) error
	IsOnline(userID string) bool
}

// UnreadClearer 未读清除接口
type UnreadClearer interface {
	ClearUnread(userID string, fromID, groupID int64) error
}

// OfflineNotifier 离线通知器接口
type OfflineNotifier interface {
	HandleUserConnect(userID string, srv ConnServer)
	ClearUnread(userID string, fromID, groupID int64) error
}

// ==================== HandlerRegistry ====================

// HandlerRegistry 处理器注册表（命令路由模式）
type HandlerRegistry struct {
	handlers           map[string]MessageHandler
	connectHandlers    []ConnectHandler
	disconnectHandlers []DisconnectHandler
}

// NewHandlerRegistry 创建处理器注册表
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]MessageHandler),
	}
}

// Register 注册指定命令的处理器
func (r *HandlerRegistry) Register(cmd string, h MessageHandler) {
	r.handlers[cmd] = h
	logger.Debug("注册消息处理器", zap.String("cmd", cmd))
}

// RegisterConnectHandler 注册连接处理器
func (r *HandlerRegistry) RegisterConnectHandler(h ConnectHandler) {
	r.connectHandlers = append(r.connectHandlers, h)
	logger.Debug("注册连接处理器", zap.Int("count", len(r.connectHandlers)))
}

// RegisterDisconnectHandler 注册断开处理器
func (r *HandlerRegistry) RegisterDisconnectHandler(h DisconnectHandler) {
	r.disconnectHandlers = append(r.disconnectHandlers, h)
	logger.Debug("注册断开处理器", zap.Int("count", len(r.disconnectHandlers)))
}

// HandleMessage 路由消息到对应的命令处理器
func (r *HandlerRegistry) HandleMessage(ctx context.Context, userID string, msg Message) {
	handler, ok := r.handlers[msg.Cmd()]
	if !ok {
		logger.Warn("未注册的命令", zap.String("cmd", msg.Cmd()))
		return
	}
	if err := handler.Handle(ctx, userID, msg); err != nil {
		logger.Warn("消息处理失败",
			zap.String("cmd", msg.Cmd()),
			zap.String("userID", userID),
			zap.Error(err))
	}
}

// HandleConnect 处理连接
func (r *HandlerRegistry) HandleConnect(ctx context.Context, userID string) {
	for _, h := range r.connectHandlers {
		if err := h.OnConnect(ctx, userID); err != nil {
			logger.Warn("连接处理失败", zap.String("userID", userID), zap.Error(err))
		}
	}
}

// HandleDisconnect 处理断开
func (r *HandlerRegistry) HandleDisconnect(ctx context.Context, userID string) {
	for _, h := range r.disconnectHandlers {
		if err := h.OnDisconnect(ctx, userID); err != nil {
			logger.Warn("断开处理失败", zap.String("userID", userID), zap.Error(err))
		}
	}
}
