package handler

import (
	"context"
	"strconv"
	"time"

	"gochat/internal/connect"
	"gochat/internal/logic/model"
	"gochat/internal/pkg/errors"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/snowflake"
	"gochat/internal/pkg/telemetry"

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
	return &ChatHandler{msgSvc: msgSvc, groupSvc: groupSvc, connSvr: connSvr}
}

// Handle 处理聊天消息
func (h *ChatHandler) Handle(ctx context.Context, userID string, msg connect.Message) error {
	raw, ok := msg.(*connect.RawMessage)
	if !ok {
		return errors.New(errors.CodeWsInvalidMsg, "消息格式无效")
	}
	chatData, err := raw.GetChatMsgData()
	if err != nil {
		logger.Warn("解析聊天消息失败", zap.String("userID", userID), zap.Error(err))
		SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsInvalidMsg, "解析消息失败"))
		return err
	}
	if err := chatData.Validate(); err != nil {
		logger.Warn("消息格式验证失败", zap.String("userID", userID), zap.Error(err))
		SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsInvalidMsg, err.Error()))
		return err
	}

	senderID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		logger.Warn("无效的用户ID", zap.String("userID", userID), zap.Error(err))
		return err
	}
	if len(msg.Seq()) > 128 {
		err := errors.New(errors.CodeWsInvalidMsg, "seq长度不能超过128")
		SendError(h.connSvr, userID, msg.Seq(), err)
		return err
	}
	if msg.Seq() != "" {
		existing, lookupErr := h.msgSvc.GetByClientSeq(senderID, msg.Seq())
		if lookupErr != nil {
			return lookupErr
		}
		if existing != nil {
			h.sendMsgAck(userID, msg.Seq(), existing.ID, true)
			return nil
		}
	}

	msgModel := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  senderID,
		ToID:    chatData.ToID,
		GroupID: chatData.GroupID,
		MsgType: chatData.Type,
		Content: chatData.Content,
	}
	if msg.Seq() != "" {
		clientSeq := msg.Seq()
		msgModel.ClientSeq = &clientSeq
	}

	var groupMembers []*model.GroupMember
	unreadTargets := make([]model.UnreadTarget, 0, 1)
	if msgModel.ToID > 0 {
		unreadTargets = append(unreadTargets, model.UnreadTarget{UserID: msgModel.ToID, FromID: msgModel.FromID})
	}
	if msgModel.GroupID > 0 {
		groupMembers, err = h.groupSvc.GetMembers(msgModel.GroupID)
		if err != nil {
			logger.Error("获取群成员失败", zap.Int64("groupID", msgModel.GroupID), zap.Error(err))
			SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsSendFailed, "获取群成员失败"))
			return err
		}
		for _, member := range groupMembers {
			if member.UserID == senderID {
				continue
			}
			unreadTargets = append(unreadTargets, model.UnreadTarget{UserID: member.UserID, GroupID: msgModel.GroupID})
		}
		canSend := false
		for _, member := range groupMembers {
			if member.UserID == senderID && (member.MutedUntil == nil || !member.MutedUntil.After(time.Now())) {
				canSend = true
				break
			}
		}
		if !canSend {
			err := errors.New(errors.CodeWsSendFailed, "group membership required or member muted")
			SendError(h.connSvr, userID, msg.Seq(), err)
			return err
		}
	}

	if err := h.msgSvc.SendWithUnread(msgModel, unreadTargets); err != nil {
		telemetry.Default.MessagePersisted(false)
		if msg.Seq() != "" {
			if existing, lookupErr := h.msgSvc.GetByClientSeq(senderID, msg.Seq()); lookupErr == nil && existing != nil {
				h.sendMsgAck(userID, msg.Seq(), existing.ID, true)
				return nil
			}
		}
		logger.Error("保存消息和未读摘要失败", zap.String("userID", userID), zap.Error(err))
		SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsSendFailed, "消息发送失败"))
		return err
	}

	telemetry.Default.MessagePersisted(true)
	logger.Debug("消息处理成功", zap.String("userID", userID), zap.Int64("messageID", msgModel.ID))
	h.sendMsgAck(userID, msg.Seq(), msgModel.ID, false)
	if msgModel.ToID > 0 {
		h.forwardToUser(strconv.FormatInt(msgModel.ToID, 10), msg)
	}
	if msgModel.GroupID > 0 {
		h.broadcastToGroup(msgModel.GroupID, groupMembers, msg, userID)
	}
	return nil
}

func (h *ChatHandler) sendMsgAck(userID, seq string, messageID int64, duplicate bool) {
	ackData := map[string]any{"status": "success", "message_id": messageID, "message_id_string": strconv.FormatInt(messageID, 10), "duplicate": duplicate}
	h.connSvr.Send(userID, connect.NewRawMessage(connect.CmdMsgReply, seq, ackData))
}

func (h *ChatHandler) forwardToUser(userID string, msg connect.Message) {
	if h.connSvr.IsOnline(userID) {
		h.connSvr.Send(userID, msg)
	}
}

func (h *ChatHandler) broadcastToGroup(groupID int64, members []*model.GroupMember, msg connect.Message, excludeUserID string) {
	for _, member := range members {
		memberID := strconv.FormatInt(member.UserID, 10)
		if memberID == excludeUserID || !h.connSvr.IsOnline(memberID) {
			continue
		}
		if err := h.connSvr.Send(memberID, msg); err != nil {
			logger.Warn("发送群消息失败", zap.String("userID", memberID), zap.Error(err))
		}
	}
	logger.Debug("群消息广播完成", zap.Int64("groupID", groupID), zap.Int("memberCount", len(members)))
}
