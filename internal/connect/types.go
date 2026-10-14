package connect

import (
	"gochat/internal/logic/service"
	"strings"
)

// AuthData 认证数据
type AuthData struct {
	Token string `json:"token"`
}

// ChatMsgData 聊天消息数据
type ChatMsgData struct {
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	Content        string `json:"content"`
}

// ErrorData 错误数据
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Validate 验证聊天消息数据
func (c *ChatMsgData) Validate() error {
	if _, _, err := service.ParseConversationScope(c.ConversationID); err != nil {
		return ErrInvalidTarget
	}
	switch c.Type {
	case "text", "image", "file", "mention", "voice", "video":
		// 所有类型 content 均不可为空
	default:
		return ErrInvalidMsgType
	}
	if strings.TrimSpace(c.Content) == "" {
		return ErrEmptyContent
	}

	return nil
}
