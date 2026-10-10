package connect

import (
	"strconv"
	"time"

	"gochat/internal/logic/model"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/logger"

	"go.uber.org/zap"
)

// offlineNotifier 离线通知管理器（实现 OfflineNotifier 接口）
type offlineNotifier struct {
	messageService *service.MessageService
}

// NewOfflineNotifier 创建离线通知管理器
func NewOfflineNotifier(messageService *service.MessageService) OfflineNotifier {
	return &offlineNotifier{
		messageService: messageService,
	}
}

// OfflineNotifyData 离线通知数据。
// 每个 unread_list 项代表一个私聊或群聊范围，而不是一条具体消息。
type OfflineNotifyData struct {
	UserID           string        `json:"user_id"`
	TotalUnreadCount int64         `json:"total_unread_count"`
	UnreadList       []UnreadCount `json:"unread_list"`
}

// UnreadCount 会话级未读消息摘要。
// 私聊使用 FromID + GroupID=0，群聊使用 FromID=0 + GroupID。
type UnreadCount struct {
	FromID        int64          `json:"from_id"`
	GroupID       int64          `json:"group_id"`
	Count         int64          `json:"count"`
	LatestMessage *LatestMessage `json:"latest_message,omitempty"`
}

// LatestMessage 离线摘要中的最新消息信息。
type LatestMessage struct {
	ID        int64      `json:"id,string"`
	SenderID  int64      `json:"sender_id"`
	Type      string     `json:"type"`
	Preview   string     `json:"preview"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// GetOfflineNotify 从数据库获取用户的会话级离线通知。
func (n *offlineNotifier) GetOfflineNotify(userID string) (*OfflineNotifyData, error) {
	userIDInt := mustParseInt64Offline(userID)

	totalUnreadCount, err := n.messageService.GetUnreadCount(userIDInt)
	if err != nil {
		return nil, err
	}
	unreadList, err := n.messageService.GetUnreadList(userIDInt)
	if err != nil {
		return nil, err
	}

	list := make([]UnreadCount, len(unreadList))
	for i, uc := range unreadList {
		list[i] = UnreadCount{
			FromID:        uc.FromID,
			GroupID:       uc.GroupID,
			Count:         uc.UnreadCount,
			LatestMessage: latestMessageFromSummary(uc),
		}
	}

	return &OfflineNotifyData{
		UserID:           userID,
		TotalUnreadCount: totalUnreadCount,
		UnreadList:       list,
	}, nil
}

// HandleUserConnect 处理用户上线，加载并发送离线通知。
func (n *offlineNotifier) HandleUserConnect(userID string, srv ConnServer) {
	notifyData, err := n.GetOfflineNotify(userID)
	if err != nil {
		logger.Warn("获取离线通知失败",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	if len(notifyData.UnreadList) == 0 {
		logger.Debug("用户无离线通知", zap.String("userID", userID))
		return
	}

	notifyMsg := NewRawMessage(CmdOfflineNotify, "", notifyData)
	if err := srv.Send(userID, notifyMsg); err != nil {
		logger.Warn("发送离线通知失败",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	logger.Info("发送离线通知成功",
		zap.String("userID", userID),
		zap.Int("conversationCount", len(notifyData.UnreadList)),
		zap.Int64("totalUnreadCount", notifyData.TotalUnreadCount))
}

// ClearUnread 清除某个私聊或群聊范围的未读计数。
func (n *offlineNotifier) ClearUnread(userID string, fromID int64, groupID int64) error {
	return n.messageService.ClearUnread(mustParseInt64Offline(userID), fromID, groupID)
}

func latestMessageFromSummary(summary *model.UnreadCount) *LatestMessage {
	if summary == nil || summary.LatestMessageID == 0 {
		return nil
	}
	return &LatestMessage{
		ID:        summary.LatestMessageID,
		SenderID:  summary.LatestSenderID,
		Type:      summary.LatestMessageType,
		Preview:   summary.LatestPreview,
		CreatedAt: summary.LatestMessageAt,
	}
}

func mustParseInt64Offline(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
