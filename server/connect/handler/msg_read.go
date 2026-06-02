package handler

import (
	"context"

	"gochat/connect"
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
	// ParseWsMsg 已提前反序列化 Data 为 *MsgReadData
	raw, ok := msg.(*connect.RawMessage)
	if !ok {
		return connect.ErrInvalidDataType
	}

	readData, ok := raw.Data.(*connect.MsgReadData)
	if !ok {
		logger.Warn("解析消息已读数据失败", zap.String("userID", userID))
		h.sendError(userID, msg.Seq(), 1001, "无效的消息ID")
		return nil
	}

	messageID := readData.MessageID
	msgModel, err := h.msgSvc.GetByID(messageID)
	if err != nil {
		logger.Warn("获取消息失败", zap.Int64("messageID", messageID), zap.Error(err))
		h.sendError(userID, msg.Seq(), 2001, "消息不存在")
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

func (h *MsgReadHandler) sendError(userID, seq string, code int, message string) {
	errorMsg := connect.NewErrorMsg(code, message)
	errorMsg.SeqStr = seq
	h.connSvr.Send(userID, errorMsg)
}
