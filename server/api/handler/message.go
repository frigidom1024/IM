package handler

import (
	"net/http"
	"strconv"

	"gochat/logic/model"
	"gochat/logic/registry"
	"gochat/logic/service"

	"github.com/gin-gonic/gin"
)

// MessageWithSender 消息 + 发送者信息
type MessageWithSender struct {
	ID          int64              `json:"id"`
	FromID      int64              `json:"from_id"`
	ToID        int64              `json:"to_id"`
	GroupID     int64              `json:"group_id"`
	MsgType     string             `json:"msg_type"`
	Content     string             `json:"content"`
	IsRevoked   bool               `json:"is_revoked"`
	CreatedAt   string             `json:"created_at"`
	SenderName  string             `json:"sender_name"`
	SenderAvata string             `json:"sender_avatar"`
	SenderID    int64              `json:"sender_id"`
}

func enrichMessages(msgs []*model.Message, userRepo registry.UserRepository) []MessageWithSender {
	seen := make(map[int64]*model.User)
	result := make([]MessageWithSender, 0, len(msgs))
	for _, m := range msgs {
		info := &MessageWithSender{
			ID:        m.ID,
			FromID:    m.FromID,
			ToID:      m.ToID,
			GroupID:   m.GroupID,
			MsgType:   m.MsgType,
			Content:   m.Content,
			IsRevoked: m.IsRevoked,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		// 查发送者信息
		user, ok := seen[m.FromID]
		if !ok {
			u, err := userRepo.GetByID(m.FromID)
			if err == nil {
				user = u
				seen[m.FromID] = u
			}
		}
		if user != nil {
			info.SenderName = user.Nickname
			if info.SenderName == "" {
				info.SenderName = user.Username
			}
			info.SenderAvata = user.Avatar
		}
		result = append(result, *info)
	}
	return result
}

// MessageHandler 消息处理器
type MessageHandler struct {
	messageService *service.MessageService
	userRepo       registry.UserRepository
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
	userRepo registry.UserRepository,
) *MessageHandler {
	return &MessageHandler{
		messageService: service.NewMessageService(messageRepo, messageReadRepo),
		userRepo:       userRepo,
	}
}

// GetPrivateMessages 获取私聊消息
func (h *MessageHandler) GetPrivateMessages(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	userIDStr := c.Param("userId")
	targetID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的用户ID"})
		return
	}

	messages, err := h.messageService.GetPrivateMessages(userID, targetID, 50, 0)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": enrichMessages(messages, h.userRepo),
	})
}

// GetGroupMessages 获取群聊消息
func (h *MessageHandler) GetGroupMessages(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	groupIDStr := c.Param("groupId")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的群组ID"})
		return
	}

	messages, err := h.messageService.GetGroupMessages(groupID, 50, 0)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": enrichMessages(messages, h.userRepo),
	})
}

// MarkRead 标记消息已读
func (h *MessageHandler) MarkRead(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req struct {
		FromID int64 `json:"from_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	if err := h.messageService.ClearUnread(userID, req.FromID, 0); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "标记成功",
	})
}

// GetUnreadCount 获取未读数量
func (h *MessageHandler) GetUnreadCount(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	count, err := h.messageService.GetUnreadCount(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"count": count,
		},
	})
}
