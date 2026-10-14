package handler

import (
	"fmt"
	"strconv"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"

	"github.com/gin-gonic/gin"
)

// MessageWithSender 消息 + 发送者信息
type MessageWithSender struct {
	ID          int64  `json:"id"`
	IDString    string `json:"id_string"`
	FromID      int64  `json:"from_id"`
	ToID        int64  `json:"to_id"`
	GroupID     int64  `json:"group_id"`
	MsgType     string `json:"msg_type"`
	Content     string `json:"content"`
	IsRevoked   bool   `json:"is_revoked"`
	CreatedAt   string `json:"created_at"`
	SenderName  string `json:"sender_name"`
	SenderAvata string `json:"sender_avatar"`
	SenderID    int64  `json:"sender_id"`
}

func enrichMessages(msgs []*model.Message, userRepo registry.UserRepository) []MessageWithSender {
	seen := make(map[int64]*model.User)
	result := make([]MessageWithSender, 0, len(msgs))
	for _, m := range msgs {
		info := &MessageWithSender{
			ID:        m.ID,
			IDString:  strconv.FormatInt(m.ID, 10),
			FromID:    m.FromID,
			ToID:      m.ToID,
			GroupID:   m.GroupID,
			MsgType:   m.MsgType,
			Content:   m.Content,
			IsRevoked: m.IsRevoked,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if m.IsRevoked {
			info.Content = ""
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

func parsePage(c *gin.Context) (int, int64, error) {
	limit := 50
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 100 {
			return 0, 0, fmt.Errorf("limit必须在1到100之间")
		}
		limit = parsed
	}
	offset := int64(0)
	if value := c.Query("offset"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset必须是非负整数")
		}
		offset = parsed
	}
	return limit, offset, nil
}
