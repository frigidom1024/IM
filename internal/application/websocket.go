package application

import (
	"gochat/internal/connect"
	"gochat/internal/connect/handler"
	"gochat/internal/logic/service"
)

func newWebSocketServer(messaging *service.MessagingService, groups *service.GroupService) *connect.WsConnServer {
	server := connect.NewWsConnServer()
	server.Registry().Register(connect.CmdMsg, handler.NewChatHandler(messaging, groups, server))
	server.Registry().Register(connect.CmdPing, handler.NewPingHandler(server))
	server.Registry().RegisterConnectHandler(handler.NewDefaultConnectHandler())
	server.Registry().RegisterDisconnectHandler(handler.NewDefaultDisconnectHandler())
	return server
}
