package connect

// 命令类型定义
const (
	// 认证相关
	CmdAuth      = "auth"       // 客户端认证
	CmdAuthReply = "auth_reply" // 认证响应

	// 消息相关
	CmdMsg      = "msg"       // 聊天消息
	CmdMsgReply = "msg_reply" // 消息确认（ACK）

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
