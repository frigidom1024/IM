package main

import (
	"gochat/connect"
	"gochat/connect/handler"
	"gochat/logic/service"
)

// NewWsServer 工厂方法 — 创建并配置完整的 WebSocket 服务器
func NewWsServer(messageService *service.MessageService, groupService *service.GroupService) *connect.WsConnServer {
	server := connect.NewWsConnServer()
	offlineNotifier := connect.NewOfflineNotifier(messageService)

	reg := server.Registry()

	// 按命令类型注册独立的处理器（OCP: 新增命令只需追加 Register 调用）
	reg.Register(connect.CmdMsg, handler.NewChatHandler(messageService, groupService, server))
	reg.Register(connect.CmdPing, handler.NewPingHandler(server))
	reg.Register(connect.CmdMsgRead, handler.NewMsgReadHandler(messageService, offlineNotifier, server))

	reg.RegisterConnectHandler(handler.NewDefaultConnectHandler(offlineNotifier, server))
	reg.RegisterDisconnectHandler(handler.NewDefaultDisconnectHandler())

	return server
}
