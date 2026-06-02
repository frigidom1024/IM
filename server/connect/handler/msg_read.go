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
func (h *MsgReadHandler) Handle(ctx context.Context, userID string, wsMsg *connect.WsMsg) error {
	messageID, err := h.parseMessageID(wsMsg)
	if err != nil {
		logger.Warn("解析消息已读数据失败", zap.String("userID", userID))
		h.sendError(userID, wsMsg.Seq, 1001, "无效的消息ID")
		return nil
	}

	msg, err := h.msgSvc.GetByID(messageID)
	if err != nil {
		logger.Warn("获取消息失败", zap.Int64("messageID", messageID), zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 2001, "消息不存在")
		return nil
	}

	if h.clearer != nil {
		h.clearer.ClearUnread(userID, msg.FromID, msg.GroupID)
	}

	ackData := map[string]any{"status": "success", "message_id": messageID}
	ackMsg := connect.NewWsMsg(connect.CmdMsgReadReply, wsMsg.Seq, ackData)
	h.connSvr.Send(userID, ackMsg)

	logger.Debug("消息已读", zap.String("userID", userID), zap.Int64("messageID", messageID))
	return nil
}

func (h *MsgReadHandler) parseMessageID(wsMsg *connect.WsMsg) (int64, error) {
	if data, ok := wsMsg.Data.(*connect.MsgReadData); ok {
		return data.MessageID, nil
	}
	if data, ok := wsMsg.Data.(map[string]interface{}); ok {
		if v, ok := data["message_id"].(float64); ok {
			return int64(v), nil
		}
	}
	return 0, connect.ErrInvalidDataType
}

func (h *MsgReadHandler) sendError(userID, seq string, code int, message string) {
	errorMsg := connect.NewErrorMsg(code, message)
	errorMsg.Seq = seq
	h.connSvr.Send(userID, errorMsg)
}
