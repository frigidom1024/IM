package connect

import "gochat/internal/pkg/errors"

var (
	ErrInvalidMsgType  = errors.New(errors.CodeWsInvalidMsg, "无效的消息类型，支持: text/image/file/voice/video/mention")
	ErrEmptyContent    = errors.New(errors.CodeWsInvalidMsg, "消息内容不能为空")
	ErrInvalidTarget   = errors.New(errors.CodeWsInvalidMsg, "消息目标无效")
	ErrUserNotOnline   = errors.New(errors.CodeWsOffline, "用户不在线")
	ErrSendQueueFull   = errors.New(errors.CodeWsQueueFull, "发送队列已满")
	ErrNilData         = errors.New(errors.CodeWsInvalidMsg, "数据为空")
	ErrInvalidDataType = errors.New(errors.CodeWsInvalidMsg, "数据类型无效")
)
