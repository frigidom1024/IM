package handler

import (
	"context"

	"gochat/connect"
	"gochat/pkg/errors"
	"gochat/pkg/logger"

	"go.uber.org/zap"
)

// MsgReadHandler 消息已读处理器（处理 CmdMsgRead）
type MsgReadHandler struct {
	msgSvc  connect.MessageService
	clearer connect.UnreadClearer
	connSvr connect.ConnServer
}

// NewMsgReadHandler 创建消息已读处理器
func NewMsgReadHandler(msgSvc connect.MessageService, clearer connect.UnreadClearer, connSvr connect.ConnServer) *MsgReadHandler {
	return &MsgReadHandler{
		msgSvc:  msgSvc,
		clearer: clearer,
		connSvr: connSvr,
	}
}

// Handle 处理消息已读
func (h *MsgReadHandler) Handle(ctx context.Context, userID string, msg connect.Message) error {
	raw, ok := msg.(*connect.RawMessage)
	if !ok {
		return errors.New(errors.CodeWsInvalidMsg, "消息格式无效")
	}

	readData, ok := raw.Data.(*connect.MsgReadData)
	if !ok {
		logger.Warn("解析消息已读数据失败", zap.String("userID", userID))
		SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsInvalidMsg, "无效的消息ID"))
		return nil
	}

	messageID := readData.MessageID
	msgModel, err := h.msgSvc.GetByID(messageID)
	if err != nil {
		logger.Warn("获取消息失败", zap.Int64("messageID", messageID), zap.Error(err))
		SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsNotFound, "消息不存在"))
		return nil
	}

	if h.clearer != nil {
		h.clearer.ClearUnread(userID, msgModel.FromID, msgModel.GroupID)
	}

	ackData := map[string]any{"status": "success", "message_id": messageID}
	ackMsg := connect.NewRawMessage(connect.CmdMsgReadReply, msg.Seq(), ackData)
	h.connSvr.Send(userID, ackMsg)

	logger.Debug("消息已读", zap.String("userID", userID), zap.Int64("messageID", messageID))
	return nil
}
