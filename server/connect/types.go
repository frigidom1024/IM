package connect

// AuthData 认证数据
type AuthData struct {
	Token string `json:"token"`
}

// ChatMsgData 聊天消息数据
type ChatMsgData struct {
	FromID  int64  `json:"from_id"`
	ToID    int64  `json:"to_id"`    // 0 表示群消息
	GroupID int64  `json:"group_id"` // 0 表示单聊
	Type    int8   `json:"type"`
	Content string `json:"content"`
}

// MsgReadData 消息已读数据
type MsgReadData struct {
	MessageID int64 `json:"message_id"`
}

// ErrorData 错误数据
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Validate 验证聊天消息数据
func (c *ChatMsgData) Validate() error {
	if c.FromID <= 0 {
		return ErrInvalidFromID
	}
	if c.Type < 1 || c.Type > 4 {
		return ErrInvalidMsgType
	}
	if c.Content == "" {
		return ErrEmptyContent
	}
	if c.ToID > 0 && c.GroupID > 0 {
		return ErrInvalidTarget
	}
	if c.ToID == 0 && c.GroupID == 0 {
		return ErrInvalidTarget
	}
	return nil
}
