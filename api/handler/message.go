package handler

import (
	"net/http"
	"strconv"

	"gochat/logic/registry"
	"gochat/logic/service"

	"github.com/gin-gonic/gin"
)

// MessageHandler 消息处理器
type MessageHandler struct {
	messageService *service.MessageService
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
) *MessageHandler {
	return &MessageHandler{
		messageService: service.NewMessageService(messageRepo, messageReadRepo),
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
		"data":  messages,
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
		"data":  messages,
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
		FromID int64 `json:"from_id"` // 发送者ID
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	// 清除该发送者的未读计数
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
