package connect

import (
	"strconv"

	"gochat/logic/service"
	"gochat/pkg/logger"

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

// OfflineNotifyData 离线通知数据
type OfflineNotifyData struct {
	UserID     string        `json:"user_id"`
	UnreadList []UnreadCount `json:"unread_list"` // 未读消息统计
}

// UnreadCount 未读消息统计
type UnreadCount struct {
	FromID  int64 `json:"from_id"`  // 发送者ID
	GroupID int64 `json:"group_id"` // 群组ID（0表示私聊）
	Count   int64 `json:"count"`    // 未读数量
}

// GetOfflineNotify 从数据库获取用户的离线通知
func (n *offlineNotifier) GetOfflineNotify(userID string) (*OfflineNotifyData, error) {
	userIDInt := mustParseInt64Offline(userID)

	unreadList, err := n.messageService.GetUnreadList(userIDInt)
	if err != nil {
		return nil, err
	}

	list := make([]UnreadCount, len(unreadList))
	for i, uc := range unreadList {
		list[i] = UnreadCount{
			FromID:  uc.FromID,
			GroupID: uc.GroupID,
			Count:   uc.Count,
		}
	}

	return &OfflineNotifyData{
		UserID:     userID,
		UnreadList: list,
	}, nil
}

// HandleUserConnect 处理用户上线，加载并发送离线通知
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

	notifyMsg := NewWsMsg(CmdOfflineNotify, "", notifyData)
	if err := srv.Send(userID, notifyMsg); err != nil {
		logger.Warn("发送离线通知失败",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	logger.Info("发送离线通知成功",
		zap.String("userID", userID),
		zap.Int("count", len(notifyData.UnreadList)))
}

// ClearUnread 清除发送者的未读计数
func (n *offlineNotifier) ClearUnread(userID string, fromID int64, groupID int64) error {
	return n.messageService.ClearUnread(mustParseInt64Offline(userID), fromID, groupID)
}

func mustParseInt64Offline(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
