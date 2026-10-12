package errors

// 通用错误码
// 格式: XXYYY (XX=模块, YYY=具体错误)
const (
	CodeSuccess     = 0
	CodeServerError = 50000 // 服务器内部错误
)

// 认证模块 401xx
const (
	CodeUnauthorized = 40100 // 未登录/token无效
)

// 请求模块 400xx
const (
	CodeBadRequest   = 40000 // 参数错误
	CodeInvalidParam = 40001 // 参数格式无效
)

// 用户模块 410xx
const (
	CodeUserExists   = 41001 // 用户已存在
	CodeUserNotFound = 41004 // 用户不存在
	CodePwdIncorrect = 41005 // 密码错误
)

// 消息模块 420xx
const (
	CodeMsgInvalid            = 42001 // 消息类型无效
	CodeMsgEmpty              = 42002 // 消息内容为空
	CodeMsgTarget             = 42003 // 消息目标无效
	CodeMsgSendFailed         = 42004 // 消息发送失败
	CodeMsgNotFound           = 42005 // 消息不存在
	CodeMsgReadFailed         = 42006 // 消息已读失败
	CodePrivateFriendRequired = 42010
	CodePrivateTextOnly       = 42011
	CodePrivateIntroUsed      = 42012
)

// 上传模块 430xx
const (
	CodeUploadMissType = 43000 // 缺少type参数
	CodeUploadBadType  = 43001 // 不支持的消息类型
	CodeUploadTooLarge = 43002 // 文件过大
	CodeUploadBadMIME  = 43003 // 不支持的文件类型
	CodeUploadReadFail = 43004 // 读取文件失败
	CodeUploadFail     = 43005 // 上传失败
)

// 好友模块 440xx
const (
	CodeFriendExists   = 44001 // 已是好友
	CodeFriendNotFound = 44004 // 好友不存在
	CodeFriendSelf     = 44005 // 不能添加自己
)

// 群组模块 450xx
const (
	CodeGroupNotFound = 45004 // 群组不存在
	CodeGroupFull     = 45010 // 群组已满
	CodeGroupMember   = 45011 // 已是群成员
)

// WebSocket 模块 490xx
const (
	CodeWsInvalidMsg = 49001 // 消息格式无效
	CodeWsSendFailed = 49002 // 消息发送失败
	CodeWsNotFound   = 49004 // 消息不存在
	CodeWsOffline    = 49005 // 用户不在线
	CodeWsQueueFull  = 49006 // 发送队列已满
)

const (
	CodeForbidden        = 40300
	CodeNotFound         = 40400
	CodeMethodNotAllowed = 40500
	CodeRateLimited      = 42900
	CodeNotReady         = 50300
)
