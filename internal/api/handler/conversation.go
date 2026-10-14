package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"
	apierrors "gochat/internal/pkg/errors"
)

type ConversationHandler struct {
	conversations *service.ConversationService
	users         registry.UserRepository
}

func NewConversationHandler(repo registry.ConversationRepository, messages registry.MessageRepository, users registry.UserRepository) *ConversationHandler {
	return &ConversationHandler{conversations: service.NewConversationService(repo, messages), users: users}
}
func ParseConversationScope(scope string) (string, int64, error) {
	return service.ParseConversationScope(scope)
}
func conversationResult(c *gin.Context, data any, err error) {
	if errors.Is(err, service.ErrConversationInvalid) {
		apierrors.RespondBadRequest(c, "无效的会话操作")
		return
	}
	groupResult(c, data, err)
}
func (h *ConversationHandler) List(c *gin.Context) {
	limit, offset, err := parsePage(c)
	if err != nil {
		apierrors.RespondBadRequest(c, err.Error())
		return
	}
	data, err := h.conversations.List(c.GetInt64("userID"), limit, offset)
	conversationResult(c, data, err)
}
func (h *ConversationHandler) Open(c *gin.Context) {
	conversationResult(c, nil, h.conversations.Open(c.GetInt64("userID"), c.Param("scope")))
}
func (h *ConversationHandler) Patch(c *gin.Context) {
	var request struct {
		Pinned *bool   `json:"pinned"`
		Muted  *bool   `json:"muted"`
		Draft  *string `json:"draft"`
	}
	if c.ShouldBindJSON(&request) != nil {
		apierrors.RespondBadRequest(c, "无效的会话偏好")
		return
	}
	conversationResult(c, nil, h.conversations.Patch(c.GetInt64("userID"), c.Param("scope"), request.Pinned, request.Muted, request.Draft))
}
func (h *ConversationHandler) MarkRead(c *gin.Context) {
	row, err := h.conversations.MarkRead(c.GetInt64("userID"), c.Param("scope"))
	if err != nil {
		conversationResult(c, nil, err)
		return
	}
	conversationResult(c, gin.H{"id": service.ConversationScope(*row), "unread_count": row.UnreadCount, "read_at": row.ReadAt}, nil)
}
func (h *ConversationHandler) Unread(c *gin.Context) {
	count, list, err := h.conversations.UnreadSummary(c.GetInt64("userID"))
	if err != nil {
		conversationResult(c, nil, err)
		return
	}
	conversationResult(c, gin.H{"count": count, "total_unread_count": count, "unread_list": list}, nil)
}
func (h *ConversationHandler) History(c *gin.Context) {
	limit, offset, err := parsePage(c)
	if err != nil {
		apierrors.RespondBadRequest(c, err.Error())
		return
	}
	messages, err := h.conversations.History(c.GetInt64("userID"), c.Param("scope"), limit, offset)
	if err != nil {
		conversationResult(c, nil, err)
		return
	}
	conversationResult(c, enrichMessages(messages, h.users), nil)
}
