package application

import (
	"gochat/internal/connect"
	"gochat/internal/connect/handler"
	"gochat/internal/logic/service"
)

func newWebSocketServer(messageService *service.MessageService, groupService *service.GroupService) *connect.WsConnServer {
	server := connect.NewWsConnServer()
	offlineNotifier := connect.NewOfflineNotifier(messageService)

	reg := server.Registry()
	reg.Register(connect.CmdMsg, handler.NewChatHandler(messageService, groupService, server))
	reg.Register(connect.CmdPing, handler.NewPingHandler(server))
	reg.Register(connect.CmdMsgRead, handler.NewMsgReadHandler(messageService, offlineNotifier, server))

	reg.RegisterConnectHandler(handler.NewDefaultConnectHandler(offlineNotifier, server))
	reg.RegisterDisconnectHandler(handler.NewDefaultDisconnectHandler())

	return server
}
