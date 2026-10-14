package connect

import (
	"encoding/json"
)

// Message 通用消息接口
// 所有 WebSocket 消息均实现此接口，按 Cmd() 路由到对应处理器
type Message interface {
	Cmd() string
	Seq() string
	ToJSON() ([]byte, error)
}

// RawMessage 通用消息体，覆盖大部分命令场景
type RawMessage struct {
	CmdStr string `json:"cmd"`
	SeqStr string `json:"seq"`
	Data   any    `json:"data"`
}

func (m *RawMessage) Cmd() string             { return m.CmdStr }
func (m *RawMessage) Seq() string             { return m.SeqStr }
func (m *RawMessage) ToJSON() ([]byte, error) { return json.Marshal(m) }

// GetChatMsgData 获取聊天消息数据
func (m *RawMessage) GetChatMsgData() (*ChatMsgData, error) {
	d, ok := m.Data.(*ChatMsgData)
	if !ok {
		return nil, ErrInvalidDataType
	}
	return d, nil
}

// GetAuthData 获取认证数据
func (m *RawMessage) GetAuthData() (*AuthData, error) {
	d, ok := m.Data.(*AuthData)
	if !ok {
		return nil, ErrInvalidDataType
	}
	return d, nil
}

// ==================== 工厂方法 ====================

// NewRawMessage 创建通用消息
func NewRawMessage(cmd, seq string, data any) *RawMessage {
	return &RawMessage{CmdStr: cmd, SeqStr: seq, Data: data}
}

// NewPongMsg 创建心跳响应
func NewPongMsg() *RawMessage {
	return NewRawMessage(CmdPong, "", nil)
}

// NewErrorMsg 创建错误消息
func NewErrorMsg(code int, message string) *RawMessage {
	return NewRawMessage(CmdError, "", ErrorData{Code: code, Message: message})
}

// ==================== 序列化 ====================

// ParseWsMsg 从 JSON 解析 WebSocket 消息
// 按命令类型提前反序列化 Data，消除运行时的 map 类型断言
func ParseWsMsg(data []byte) (Message, error) {
	var raw struct {
		Cmd  string          `json:"cmd"`
		Seq  string          `json:"seq"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	msg := NewRawMessage(raw.Cmd, raw.Seq, nil)

	switch raw.Cmd {
	case CmdMsg:
		var d ChatMsgData
		if err := json.Unmarshal(raw.Data, &d); err != nil {
			return nil, err
		}
		msg.Data = &d
	case CmdAuth:
		var d AuthData
		if err := json.Unmarshal(raw.Data, &d); err != nil {
			return nil, err
		}
		msg.Data = &d
	default:
		// 未知/通用命令保持原始 RawMessage
		msg.Data = raw.Data
	}

	return msg, nil
}
