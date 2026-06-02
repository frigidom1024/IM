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

	registry := server.Registry()
	registry.SetMessageHandler(handler.NewDefaultMessageHandler(
		messageService, groupService, offlineNotifier, server,
	))
	registry.SetConnectHandler(handler.NewDefaultConnectHandler(offlineNotifier, server))
	registry.SetDisconnectHandler(handler.NewDefaultDisconnectHandler())

	return server
}
