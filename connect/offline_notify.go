package connect

import (
	"strconv"

	"gochat/logic/model"
	"gochat/logic/service"
	"gochat/pkg/logger"

	"go.uber.org/zap"
)

// OfflineNotifier 离线通知管理器
type OfflineNotifier struct {
	messageService *service.MessageService
}

// NewOfflineNotifier 创建离线通知管理器
func NewOfflineNotifier(messageService *service.MessageService) *OfflineNotifier {
	return &OfflineNotifier{
		messageService: messageService,
	}
}

// OfflineNotifyData 离线通知数据
type OfflineNotifyData struct {
	UserID     string          `json:"user_id"`
	Messages   []*model.Message `json:"messages"`   // 未读消息列表
	TotalCount int64           `json:"total_count"` // 总未读数
}

// GetOfflineNotify 从数据库获取用户的离线通知
func (n *OfflineNotifier) GetOfflineNotify(userID string) (*OfflineNotifyData, error) {
	userIDInt := mustParseInt64Offline(userID)

	// 1. 获取未读消息数量
	totalCount, err := n.messageService.GetUnreadCount(userIDInt)
	if err != nil {
		return nil, err
	}

	// 2. 获取最近的未读消息（限制数量，避免数据过大）
	messages, err := n.messageService.GetUnreadMessages(userIDInt, 50)
	if err != nil {
		return nil, err
	}

	return &OfflineNotifyData{
		UserID:     userID,
		Messages:   messages,
		TotalCount: totalCount,
	}, nil
}

// MarkRead 标记消息已读
func (n *OfflineNotifier) MarkRead(userID string, msgID int64, msg *model.Message) error {
	// 直接持久化到数据库
	return n.messageService.MarkRead(msgID, mustParseInt64Offline(userID))
}

// HandleUserConnect 处理用户上线，加载并发送离线通知
func (n *OfflineNotifier) HandleUserConnect(userID string, server *WsConnServer) {
	// 1. 从数据库获取离线通知
	notifyData, err := n.GetOfflineNotify(userID)
	if err != nil {
		logger.Warn("获取离线通知失败",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	// 2. 如果没有离线通知，直接返回
	if notifyData.TotalCount == 0 {
		logger.Debug("用户无离线通知", zap.String("userID", userID))
		return
	}

	// 3. 发送离线通知
	if err := server.SendOfflineNotify(userID, notifyData); err != nil {
		logger.Warn("发送离线通知失败",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	logger.Info("发送离线通知成功",
		zap.String("userID", userID),
		zap.Int64("count", notifyData.TotalCount))
}

// mustParseInt64Offline 解析字符串为int64，失败返回0
func mustParseInt64Offline(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
