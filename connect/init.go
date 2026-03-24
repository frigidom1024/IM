package connect

import (
	"gochat/logic/service"
)

// InitServer 初始化连接服务器并设置所有必要的处理器
func InitServer(
	messageService *service.MessageService,
	groupService *service.GroupService,
) *WsConnServer {
	// 1. 创建 WebSocket 服务器
	server := NewWsConnServer()
	registry := server.Registry()

	// 2. 创建离线通知管理器
	offlineNotifier := NewOfflineNotifier(messageService)

	// 3. 设置消息处理器
	messageHandler := NewDefaultMessageHandler(
		messageService,
		groupService,
		offlineNotifier,
		server,
	)
	registry.SetMessageHandler(messageHandler)

	// 4. 设置连接处理器
	connectHandler := NewDefaultConnectHandler(offlineNotifier, server)
	registry.SetConnectHandler(connectHandler)

	// 5. 设置断开处理器
	disconnectHandler := NewDefaultDisconnectHandler()
	registry.SetDisconnectHandler(disconnectHandler)

	return server
}
