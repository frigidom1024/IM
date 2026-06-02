package connect

import (
	"encoding/json"
	"errors"
)

// 错误定义
var (
	ErrInvalidFromID   = errors.New("发送者ID无效")
	ErrInvalidMsgType  = errors.New("消息类型无效")
	ErrEmptyContent    = errors.New("消息内容不能为空")
	ErrInvalidTarget   = errors.New("消息目标无效")
	ErrUserNotOnline   = errors.New("用户不在线")
	ErrSendQueueFull   = errors.New("发送队列已满")
	ErrNilData         = errors.New("数据为空")
	ErrInvalidDataType = errors.New("数据类型无效")
)

// WsMsg WebSocket 通用消息结构
type WsMsg struct {
	Cmd string `json:"cmd"` // 命令类型
	Seq string `json:"seq"` // 序列号（用于请求响应匹配）

	Data any `json:"data"` // 数据（具体内容根据 Cmd 变化）
}

// 命令类型定义
const (
	// 认证相关
	CmdAuth      = "auth"       // 客户端认证
	CmdAuthReply = "auth_reply" // 认证响应

	// 消息相关
	CmdMsg           = "msg"             // 聊天消息
	CmdMsgReply      = "msg_reply"       // 消息确认（ACK）
	CmdMsgRead       = "msg_read"        // 消息已读
	CmdMsgReadReply  = "msg_read_reply"  // 已读确认
	CmdOfflineNotify = "offline_notify"  // 离线通知

	// 状态相关
	CmdOnline  = "online"  // 用户上线
	CmdOffline = "offline" // 用户离线
	CmdTyping  = "typing"  // 正在输入
	CmdState   = "state"   // 状态变更

	// 错误
	CmdError = "error" // 错误响应
	CmdPing  = "ping"  // 心跳
	CmdPong  = "pong"  // 心跳响应
)

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

// StateData 状态数据
type StateData struct {
	Status int8 `json:"status"` // 0离线 1在线 2忙碌
}

// ErrorData 错误数据
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewWsMsg 创建 WebSocket 消息
func NewWsMsg(cmd, seq string, data any) *WsMsg {
	return &WsMsg{
		Cmd:  cmd,
		Seq:  seq,
		Data: data,
	}
}

// NewAuthMsg 创建认证消息
func NewAuthMsg(token string) *WsMsg {
	return &WsMsg{
		Cmd:  CmdAuth,
		Seq:  "",
		Data: AuthData{Token: token},
	}
}

// NewChatMsg 创建聊天消息
func NewChatMsg(fromID, toID, groupID int64, msgType int8, content string) *WsMsg {
	return &WsMsg{
		Cmd: CmdMsg,
		Seq: "",
		Data: ChatMsgData{
			FromID:  fromID,
			ToID:    toID,
			GroupID: groupID,
			Type:    msgType,
			Content: content,
		},
	}
}

// NewErrorMsg 创建错误消息
func NewErrorMsg(code int, message string) *WsMsg {
	return &WsMsg{
		Cmd:  CmdError,
		Seq:  "",
		Data: ErrorData{Code: code, Message: message},
	}
}

// NewPongMsg 创建心跳响应
func NewPongMsg() *WsMsg {
	return &WsMsg{
		Cmd:  CmdPong,
		Seq:  "",
		Data: nil,
	}
}

// ToJSON 序列化为 JSON
func (m *WsMsg) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// ParseWsMsg 从 JSON 解析 WebSocket 消息
func ParseWsMsg(data []byte) (*WsMsg, error) {
	var msg WsMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// GetChatMsgData 获取聊天消息数据
func (m *WsMsg) GetChatMsgData() (*ChatMsgData, error) {
	if m.Data == nil {
		return nil, ErrNilData
	}
	// 处理 map[string]interface{} 类型
	if data, ok := m.Data.(map[string]interface{}); ok {
		result := &ChatMsgData{}
		if v, ok := data["from_id"].(float64); ok {
			result.FromID = int64(v)
		}
		if v, ok := data["to_id"].(float64); ok {
			result.ToID = int64(v)
		}
		if v, ok := data["group_id"].(float64); ok {
			result.GroupID = int64(v)
		}
		if v, ok := data["type"].(float64); ok {
			result.Type = int8(v)
		}
		if v, ok := data["content"].(string); ok {
			result.Content = v
		}
		return result, nil
	}
	// 处理直接是 ChatMsgData 类型
	if chatData, ok := m.Data.(*ChatMsgData); ok {
		return chatData, nil
	}
	// 处理 JSON 编码的字节
	if bytes, ok := m.Data.([]byte); ok {
		var result ChatMsgData
		if err := json.Unmarshal(bytes, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	return nil, ErrInvalidDataType
}

// GetAuthData 获取认证数据
func (m *WsMsg) GetAuthData() (*AuthData, error) {
	if m.Data == nil {
		return nil, ErrNilData
	}
	if data, ok := m.Data.(*AuthData); ok {
		return data, nil
	}
	if data, ok := m.Data.(map[string]interface{}); ok {
		result := &AuthData{}
		if v, ok := data["token"].(string); ok {
			result.Token = v
		}
		return result, nil
	}
	return nil, ErrInvalidDataType
}

// GetData 获取原始数据
func (m *WsMsg) GetData() (any, error) {
	return m.Data, nil
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
	// 私聊和群聊互斥
	if c.ToID > 0 && c.GroupID > 0 {
		return ErrInvalidTarget
	}
	// 必须指定接收方
	if c.ToID == 0 && c.GroupID == 0 {
		return ErrInvalidTarget
	}
	return nil
}
