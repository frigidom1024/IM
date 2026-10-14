package handler

import (
	"context"
	stdErrors "errors"
	"strconv"
	"time"

	"gochat/internal/connect"
	"gochat/internal/logic/model"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/errors"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/snowflake"
	"gochat/internal/pkg/telemetry"

	"go.uber.org/zap"
)

// ChatHandler 聊天消息处理器（处理 CmdMsg）
type ChatHandler struct {
	msgSvc   connect.MessageSender
	groupSvc connect.GroupService
	connSvr  connect.ConnServer
}

// NewChatHandler 创建聊天消息处理器
func NewChatHandler(msgSvc connect.MessageSender, groupSvc connect.GroupService, connSvr connect.ConnServer) *ChatHandler {
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
	key, err := service.ConversationKey(senderID, chatData.ConversationID)
	if err != nil {
		SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsInvalidMsg, "无效的会话标识"))
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
			return h.ackExisting(userID, msg.Seq(), senderID, chatData, existing)
		}
	}

	msgModel := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  senderID,
		ToID:    key.ToID,
		GroupID: key.GroupID,
		MsgType: chatData.Type,
		Content: chatData.Content,
	}
	if msg.Seq() != "" {
		clientSeq := msg.Seq()
		msgModel.ClientSeq = &clientSeq
	}

	var groupMembers []*model.GroupMember
	if msgModel.GroupID > 0 {
		groupMembers, err = h.groupSvc.GetMembers(msgModel.GroupID)
		if err != nil {
			logger.Error("获取群成员失败", zap.Int64("groupID", msgModel.GroupID), zap.Error(err))
			SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsSendFailed, "获取群成员失败"))
			return err
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

	recipients, err := h.msgSvc.Send(msgModel)
	if err != nil {
		telemetry.Default.MessagePersisted(false)
		if msg.Seq() != "" {
			if existing, lookupErr := h.msgSvc.GetByClientSeq(senderID, msg.Seq()); lookupErr == nil && existing != nil {
				return h.ackExisting(userID, msg.Seq(), senderID, chatData, existing)
			}
		}
		logger.Error("保存消息和未读摘要失败", zap.String("userID", userID), zap.Error(err))
		var business *errors.Error
		if stdErrors.As(err, &business) {
			SendError(h.connSvr, userID, msg.Seq(), business)
		} else {
			SendError(h.connSvr, userID, msg.Seq(), errors.New(errors.CodeWsSendFailed, "消息发送失败"))
		}
		return err
	}

	telemetry.Default.MessagePersisted(true)
	logger.Debug("消息处理成功", zap.String("userID", userID), zap.Int64("messageID", msgModel.ID))
	h.sendMsgAck(userID, msg.Seq(), senderID, msgModel, false)
	for _, recipient := range recipients {
		delivery := connect.NewRawMessage(connect.CmdMsg, msg.Seq(), map[string]any{
			"id": strconv.FormatInt(msgModel.ID, 10), "conversation_id": messageScope(msgModel, recipient), "sender_id": strconv.FormatInt(senderID, 10), "type": msgModel.MsgType, "content": msgModel.Content, "created_at": msgModel.CreatedAt,
		})
		h.forwardToUser(strconv.FormatInt(recipient, 10), delivery)
	}
	return nil
}

func messageScope(message *model.Message, viewer int64) string {
	if message.GroupID > 0 {
		return service.ConversationScope(model.Conversation{GroupID: message.GroupID})
	}
	peer := message.FromID
	if viewer == message.FromID {
		peer = message.ToID
	}
	return service.ConversationScope(model.Conversation{ToID: peer})
}
func (h *ChatHandler) ackExisting(userID, seq string, viewer int64, data *connect.ChatMsgData, message *model.Message) error {
	if messageScope(message, viewer) != data.ConversationID || message.MsgType != data.Type || message.Content != data.Content {
		err := errors.New(errors.CodeWsInvalidMsg, "seq已用于其他消息，请保留原始会话与内容")
		SendError(h.connSvr, userID, seq, err)
		return err
	}
	h.sendMsgAck(userID, seq, viewer, message, true)
	return nil
}
func (h *ChatHandler) sendMsgAck(userID, seq string, viewer int64, message *model.Message, duplicate bool) {
	ackData := map[string]any{"status": "success", "conversation_id": messageScope(message, viewer), "message_id": message.ID, "message_id_string": strconv.FormatInt(message.ID, 10), "duplicate": duplicate}
	h.connSvr.Send(userID, connect.NewRawMessage(connect.CmdMsgReply, seq, ackData))
}

func (h *ChatHandler) forwardToUser(userID string, msg connect.Message) {
	if h.connSvr.IsOnline(userID) {
		h.connSvr.Send(userID, msg)
	}
}
