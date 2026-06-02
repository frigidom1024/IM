package connect

import "gochat/logic/service"

// InitServer 初始化连接服务器，返回 server 和 offlineNotifier
// 处理器注册由 main.go 通过 connect/handler 包完成
func InitServer(messageService *service.MessageService) (*WsConnServer, *OfflineNotifier) {
	server := NewWsConnServer()
	offlineNotifier := NewOfflineNotifier(messageService)
	return server, offlineNotifier
}
