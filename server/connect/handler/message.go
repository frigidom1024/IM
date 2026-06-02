package handler

import (
	"context"
	"strconv"

	"gochat/connect"
	"gochat/logic/model"
	"gochat/logic/service"
	"gochat/pkg/logger"
	"gochat/pkg/snowflake"

	"go.uber.org/zap"
)

// DefaultMessageHandler 默认消息处理器
type DefaultMessageHandler struct {
	messageService  *service.MessageService
	groupService    *service.GroupService
	offlineNotifier *connect.OfflineNotifier
	server          *connect.WsConnServer
}

// NewDefaultMessageHandler 创建默认消息处理器
func NewDefaultMessageHandler(
	messageService *service.MessageService,
	groupService *service.GroupService,
	offlineNotifier *connect.OfflineNotifier,
	server *connect.WsConnServer,
) *DefaultMessageHandler {
	return &DefaultMessageHandler{
		messageService:  messageService,
		groupService:    groupService,
		offlineNotifier: offlineNotifier,
		server:          server,
	}
}

// Handle 处理消息（内部路由）
func (h *DefaultMessageHandler) Handle(ctx context.Context, userID string, msg *connect.WsMsg) error {
	switch msg.Cmd {
	case connect.CmdMsg:
		return h.handleChatMessage(ctx, userID, msg)
	case connect.CmdPing:
		return h.handlePing(ctx, userID, msg)
	case connect.CmdMsgRead:
		return h.handleMsgRead(ctx, userID, msg)
	default:
		logger.Warn("未知命令类型",
			zap.String("cmd", msg.Cmd),
			zap.String("userID", userID))
		return nil
	}
}

// handleChatMessage 处理聊天消息
func (h *DefaultMessageHandler) handleChatMessage(ctx context.Context, userID string, wsMsg *connect.WsMsg) error {
	chatData, err := wsMsg.GetChatMsgData()
	if err != nil {
		logger.Warn("解析聊天消息失败",
			zap.String("userID", userID),
			zap.Error(err))
		return err
	}

	if err := chatData.Validate(); err != nil {
		logger.Warn("消息格式验证失败",
			zap.String("userID", userID),
			zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 1001, err.Error())
		return err
	}

	senderID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		logger.Warn("无效的用户ID",
			zap.String("userID", userID),
			zap.Error(err))
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

	if err := h.messageService.Send(msg); err != nil {
		logger.Error("保存消息失败",
			zap.String("userID", userID),
			zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 2001, "消息发送失败")
		return err
	}

	if msg.ToID > 0 {
		if err := h.messageService.IncrUnread(msg.ToID, msg.FromID, 0); err != nil {
			logger.Warn("增加未读计数失败",
				zap.Int64("userID", msg.ToID),
				zap.Int64("fromID", msg.FromID),
				zap.Error(err))
		}
	}

	logger.Debug("消息处理成功",
		zap.String("userID", userID),
		zap.Int64("messageID", msg.ID))

	h.sendMsgAck(userID, wsMsg.Seq, msg.ID)

	if msg.ToID > 0 {
		h.forwardToUser(strconv.FormatInt(msg.ToID, 10), wsMsg)
	}

	if msg.GroupID > 0 {
		h.broadcastToGroup(msg.GroupID, wsMsg, userID)
	}

	return nil
}

// handlePing 处理心跳
func (h *DefaultMessageHandler) handlePing(ctx context.Context, userID string, wsMsg *connect.WsMsg) error {
	logger.Debug("处理心跳", zap.String("userID", userID))
	pongMsg := connect.NewPongMsg()
	err := h.server.Send(userID, pongMsg)
	if err != nil {
		logger.Error("发送pong失败", zap.String("userID", userID), zap.Error(err))
	} else {
		logger.Debug("发送pong成功", zap.String("userID", userID))
	}
	return err
}

// handleMsgRead 处理消息已读
func (h *DefaultMessageHandler) handleMsgRead(ctx context.Context, userID string, wsMsg *connect.WsMsg) error {
	var readData *connect.MsgReadData
	if data, ok := wsMsg.Data.(*connect.MsgReadData); ok {
		readData = data
	} else if data, ok := wsMsg.Data.(map[string]interface{}); ok {
		readData = &connect.MsgReadData{}
		if v, ok := data["message_id"].(float64); ok {
			readData.MessageID = int64(v)
		} else {
			logger.Warn("解析消息已读数据失败", zap.String("userID", userID))
			h.sendError(userID, wsMsg.Seq, 1001, "无效的消息ID")
			return nil
		}
	} else {
		logger.Warn("解析消息已读数据失败", zap.String("userID", userID))
		h.sendError(userID, wsMsg.Seq, 1001, "无效的消息已读数据")
		return nil
	}

	msg, err := h.messageService.GetByID(readData.MessageID)
	if err != nil {
		logger.Warn("获取消息失败", zap.Int64("messageID", readData.MessageID), zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 2001, "消息不存在")
		return nil
	}

	if h.offlineNotifier != nil {
		h.offlineNotifier.ClearUnread(userID, msg.FromID, msg.GroupID)
	}

	ackData := map[string]any{
		"status":     "success",
		"message_id": readData.MessageID,
	}
	ackMsg := connect.NewWsMsg(connect.CmdMsgReadReply, wsMsg.Seq, ackData)
	h.sendToUser(userID, ackMsg)

	logger.Debug("消息已读", zap.String("userID", userID), zap.Int64("messageID", readData.MessageID))
	return nil
}

// sendError 发送错误消息
func (h *DefaultMessageHandler) sendError(userID string, seq string, code int, message string) {
	errorMsg := connect.NewErrorMsg(code, message)
	errorMsg.Seq = seq
	h.sendToUser(userID, errorMsg)
}

// sendMsgAck 发送消息确认
func (h *DefaultMessageHandler) sendMsgAck(userID string, seq string, messageID int64) {
	ackData := map[string]any{
		"status":     "success",
		"message_id": messageID,
	}
	ackMsg := connect.NewWsMsg(connect.CmdMsgReply, seq, ackData)
	h.sendToUser(userID, ackMsg)
}

// sendToUser 向指定用户发送消息
func (h *DefaultMessageHandler) sendToUser(userID string, msg *connect.WsMsg) {
	if h.server != nil {
		h.server.Send(userID, msg)
	}
}

// forwardToUser 转发消息给指定用户
func (h *DefaultMessageHandler) forwardToUser(userID string, msg *connect.WsMsg) {
	if h.server == nil {
		return
	}
	if h.server.IsOnline(userID) {
		h.server.Send(userID, msg)
	}
}

// broadcastToGroup 向群组广播消息
func (h *DefaultMessageHandler) broadcastToGroup(groupID int64, msg *connect.WsMsg, excludeUserID string) {
	if h.server == nil {
		return
	}

	members, err := h.groupService.GetMembers(groupID)
	if err != nil {
		logger.Error("获取群成员失败",
			zap.Int64("groupID", groupID),
			zap.Error(err))
		return
	}

	for _, member := range members {
		memberID := strconv.FormatInt(member.UserID, 10)
		if memberID == excludeUserID {
			continue
		}

		fromID := mustParseInt64(excludeUserID)
		if err := h.messageService.IncrUnread(member.UserID, fromID, groupID); err != nil {
			logger.Warn("增加群成员未读计数失败",
				zap.Int64("memberID", member.UserID),
				zap.Int64("groupID", groupID),
				zap.Error(err))
		}

		if !h.server.IsOnline(memberID) {
			continue
		}

		if err := h.server.Send(memberID, msg); err != nil {
			logger.Warn("发送群消息失败",
				zap.String("userID", memberID),
				zap.Error(err))
		}
	}

	logger.Debug("群消息广播完成",
		zap.Int64("groupID", groupID),
		zap.Int("memberCount", len(members)),
		zap.String("excludeUserID", excludeUserID))
}

// mustParseInt64 解析字符串为int64
func mustParseInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
