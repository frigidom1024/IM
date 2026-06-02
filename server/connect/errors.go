package connect

import "errors"

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
