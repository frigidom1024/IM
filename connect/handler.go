package connect

import (
	"gochat/logic/model"
	"gochat/logic/service"
	"gochat/pkg/logger"
	"gochat/pkg/snowflake"
	"strconv"

	"go.uber.org/zap"
)

// Handler 消息处理器
type Handler struct {
	userService    *service.UserService
	friendService  *service.FriendService
	messageService *service.MessageService
	groupService   *service.GroupService
	server         *WsConnServer // WebSocket 服务器
}

// NewHandler 创建消息处理器
func NewHandler(
	userService *service.UserService,
	friendService *service.FriendService,
	messageService *service.MessageService,
	groupService *service.GroupService,
	server *WsConnServer,
) *Handler {
	return &Handler{
		userService:    userService,
		friendService:  friendService,
		messageService: messageService,
		groupService:   groupService,
		server:         server,
	}
}

// HandleMessage 处理接收到的 WebSocket 消息
func (h *Handler) HandleMessage(userID string, wsMsg *WsMsg) {
	switch wsMsg.Cmd {
	case CmdMsg:
		h.handleChatMessage(userID, wsMsg)
	case CmdPing:
		h.handlePing(userID, wsMsg)
	default:
		logger.Warn("未知命令类型/服务器暂未实现",
			zap.String("cmd", wsMsg.Cmd),
			zap.String("userID", userID))
	}
}

// handleChatMessage 处理聊天消息
func (h *Handler) handleChatMessage(userID string, wsMsg *WsMsg) {
	// 解析聊天消息数据
	chatData, err := wsMsg.GetChatMsgData()
	if err != nil {
		logger.Warn("解析聊天消息失败",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	// 验证消息格式
	if err := chatData.Validate(); err != nil {
		logger.Warn("消息格式验证失败",
			zap.String("userID", userID),
			zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 1001, err.Error())
		return
	}

	// 解析发送者ID
	senderID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		logger.Warn("无效的用户ID",
			zap.String("userID", userID),
			zap.Error(err))
		return
	}

	// 创建消息模型
	msg := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  senderID,
		ToID:    chatData.ToID,
		GroupID: chatData.GroupID,
		MsgType: chatData.Type,
		Content: chatData.Content,
	}

	// 保存消息
	if err := h.messageService.Send(msg); err != nil {
		logger.Error("保存消息失败",
			zap.String("userID", userID),
			zap.Error(err))
		h.sendError(userID, wsMsg.Seq, 2001, "消息发送失败")
		return
	}

	logger.Debug("消息处理成功",
		zap.String("userID", userID),
		zap.Int64("messageID", msg.ID))

	// 发送消息确认
	h.sendMsgAck(userID, wsMsg.Seq, msg.ID)

	// 如果是私聊消息，转发给接收者
	if msg.ToID > 0 {
		h.forwardToUser(strconv.FormatInt(msg.ToID, 10), wsMsg)
	}

	// 如果是群聊消息，广播给群成员
	if msg.GroupID > 0 {
		h.broadcastToGroup(msg.GroupID, wsMsg, userID)
	}
}

// handlePing 处理心跳
func (h *Handler) handlePing(userID string, wsMsg *WsMsg) {
	pongMsg := NewPongMsg()
	h.sendToUser(userID, pongMsg)
}

// sendError 发送错误消息
func (h *Handler) sendError(userID string, seq string, code int, message string) {
	errorMsg := NewErrorMsg(code, message)
	errorMsg.Seq = seq
	h.sendToUser(userID, errorMsg)
}

// sendMsgAck 发送消息确认
func (h *Handler) sendMsgAck(userID string, seq string, messageID int64) {
	ackData := map[string]any{
		"status":     "success",
		"message_id": messageID,
	}
	ackMsg := NewWsMsg(CmdMsgReply, seq, ackData)
	h.sendToUser(userID, ackMsg)
}

// sendToUser 向指定用户发送消息
func (h *Handler) sendToUser(userID string, msg *WsMsg) {
	if h.server != nil {
		h.server.Send(userID, msg)
	}
}

// forwardToUser 转发消息给指定用户
func (h *Handler) forwardToUser(userID string, msg *WsMsg) {
	if h.server != nil {
		h.server.Send(userID, msg)
	}
}

// broadcastToGroup 向群组广播消息
func (h *Handler) broadcastToGroup(groupID int64, msg *WsMsg, excludeUserID string) {
	if h.server == nil {
		return
	}

	// 查询群成员列表
	members, err := h.groupService.GetMembers(groupID)
	if err != nil {
		logger.Error("获取群成员失败",
			zap.Int64("groupID", groupID),
			zap.Error(err))
		return
	}

	// 向除发送者外的所有成员广播消息
	for _, member := range members {
		memberID := strconv.FormatInt(member.UserID, 10)
		if memberID == excludeUserID {
			continue
		}

		// 检查成员是否在线
		if !h.server.IsOnline(memberID) {
			continue
		}

		// 发送消息
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
