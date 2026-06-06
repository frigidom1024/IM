package handler

import (
	"gochat/connect"
	"gochat/pkg/errors"
)

// SendError 向用户发送 WebSocket 错误消息
// 统一封装 ErrorData + Seq，消除重复的 sendError 方法
func SendError(connSvr connect.ConnServer, userID, seq string, bizErr *errors.Error) {
	errorMsg := connect.NewErrorMsg(bizErr.Code, bizErr.Message)
	errorMsg.SeqStr = seq
	connSvr.Send(userID, errorMsg)
}
