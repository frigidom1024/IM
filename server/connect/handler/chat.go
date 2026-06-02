package handler

import (
	"context"
	"strconv"

	"gochat/connect"
	"gochat/logic/model"
	"gochat/pkg/logger"
	"gochat/pkg/snowflake"

	"go.uber.org/zap"
)

// ChatHandler 聊天消息处理器（处理 CmdMsg）
type ChatHandler struct {
	msgSvc   connect.MessageService
	groupSvc connect.GroupService
	connSvr  connect.ConnServer
}

// NewChatHandler 创建聊天消息处理器
func NewChatHandler(msgSvc connect.MessageService, groupSvc connect.GroupService, connSvr connect.ConnServer) *ChatHandler {
	return &ChatHandler{
		msgSvc:   msgSvc,
		groupSvc: groupSvc,
		connSvr:  connSvr,
	}
}

// Handle 处理聊天消息
func (h *ChatHandler) Handle(ctx context.Context, userID string, wsMsg *connect.WsMsg) error {
	chatData, err := wsMsg.GetChatMsgData()
	if err != nil {
		logger.Warn("解析聊天消息失败", zap.String("userID", userID), zap.Error(err))
		return err
	}

	if err := chatData.Validate(); err != nil {
		logger.Warn("消息格式验证失败", zap.String("userID", userID), zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 1001, err.Error())
		return err
	}

	senderID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		logger.Warn("无效的用户ID", zap.String("userID", userID), zap.Error(err))
		return err
	}

	msg := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  senderID,
		ToID:    chatData.ToID,
		GroupID: chatData.GroupID,
		MsgType: chatData.Type,
		Content: chatData.Content,
	}

	if err := h.msgSvc.Send(msg); err != nil {
		logger.Error("保存消息失败", zap.String("userID", userID), zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 2001, "消息发送失败")
		return err
	}

	// 私聊未读计数
	if msg.ToID > 0 {
		if err := h.msgSvc.IncrUnread(msg.ToID, msg.FromID, 0); err != nil {
			logger.Warn("增加未读计数失败",
				zap.Int64("userID", msg.ToID),
				zap.Int64("fromID", msg.FromID),
				zap.Error(err))
		}
	}

	logger.Debug("消息处理成功", zap.String("userID", userID), zap.Int64("messageID", msg.ID))

	h.sendMsgAck(userID, wsMsg.Seq, msg.ID)

	// 转发私聊
	if msg.ToID > 0 {
		h.forwardToUser(strconv.FormatInt(msg.ToID, 10), wsMsg)
	}

	// 群聊广播
	if msg.GroupID > 0 {
		h.broadcastToGroup(msg.GroupID, wsMsg, userID)
	}

	return nil
}

func (h *ChatHandler) sendError(userID, seq string, code int, message string) {
	errorMsg := connect.NewErrorMsg(code, message)
	errorMsg.Seq = seq
	h.connSvr.Send(userID, errorMsg)
}

func (h *ChatHandler) sendMsgAck(userID, seq string, messageID int64) {
	ackData := map[string]any{"status": "success", "message_id": messageID}
	ackMsg := connect.NewWsMsg(connect.CmdMsgReply, seq, ackData)
	h.connSvr.Send(userID, ackMsg)
}

func (h *ChatHandler) forwardToUser(userID string, msg *connect.WsMsg) {
	if h.connSvr.IsOnline(userID) {
		h.connSvr.Send(userID, msg)
	}
}

func (h *ChatHandler) broadcastToGroup(groupID int64, msg *connect.WsMsg, excludeUserID string) {
	members, err := h.groupSvc.GetMembers(groupID)
	if err != nil {
		logger.Error("获取群成员失败", zap.Int64("groupID", groupID), zap.Error(err))
		return
	}

	for _, member := range members {
		memberID := strconv.FormatInt(member.UserID, 10)
		if memberID == excludeUserID {
			continue
		}

		fromID := mustParseInt64(excludeUserID)
		if err := h.msgSvc.IncrUnread(member.UserID, fromID, groupID); err != nil {
			logger.Warn("增加群成员未读计数失败",
				zap.Int64("memberID", member.UserID),
				zap.Int64("groupID", groupID),
				zap.Error(err))
		}

		if !h.connSvr.IsOnline(memberID) {
			continue
		}

		if err := h.connSvr.Send(memberID, msg); err != nil {
			logger.Warn("发送群消息失败", zap.String("userID", memberID), zap.Error(err))
		}
	}

	logger.Debug("群消息广播完成", zap.Int64("groupID", groupID), zap.Int("memberCount", len(members)))
}

func mustParseInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
