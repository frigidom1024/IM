package connect

import (
	"gochat/pkg/logger"
	"gochat/pkg/util"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 先放开，生产要校验
	},
}

// wsConnection WebSocket连接
type wsConnection struct {
	conn      *websocket.Conn
	userID    string
	send      chan []byte
	server    *WsConnServer
	closeOnce sync.Once
}

// WsConnServer WebSocket连接服务器实现
type WsConnServer struct {
	// 用户ID到连接的映射（并发安全）
	conns sync.Map // string(userID) -> *wsConnection

	// 事件回调（支持多个）
	messageHandlers    []func(userID string, msg *WsMsg)
	connectHandlers    []func(userID string)
	disconnectHandlers []func(userID string)

	// 用于在Handler回调中保护handlers的读写
	handlersMu sync.RWMutex
}

// NewWsConnServer 创建新的WebSocket服务器
func NewWsConnServer() *WsConnServer {
	return &WsConnServer{
		messageHandlers:    make([]func(userID string, msg *WsMsg), 0),
		connectHandlers:    make([]func(userID string), 0),
		disconnectHandlers: make([]func(userID string), 0),
	}
}

// HandleWebSocket 处理WebSocket连接请求
func (s *WsConnServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从请求头获取token
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		logger.Warn("WebSocket连接失败: 缺少token")
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	// 去除 Bearer 前缀
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	// 验证token并获取用户信息
	claims, err := util.ParseToken(token)
	if err != nil {
		logger.Warn("Token验证失败", zap.Error(err))
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	userID := strconv.Itoa(claims.UserID)

	// 检查用户是否已在线（单点登录）
	if _, exists := s.conns.Load(userID); exists {
		logger.Info("踢出旧连接", zap.String("userID", userID))
		s.Disconnect(userID)
	}

	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn("WebSocket升级失败", zap.Error(err))
		return
	}

	// 创建连接对象
	wsConn := &wsConnection{
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, 256),
		server: s,
	}

	// 存储连接
	s.conns.Store(userID, wsConn)
	logger.Info("用户连接成功", zap.String("userID", userID))

	// 触发连接回调
	s.handlersMu.RLock()
	handlers := s.connectHandlers
	s.handlersMu.RUnlock()
	for _, handler := range handlers {
		go handler(userID)
	}

	// 启动读写goroutine
	go wsConn.readPump()
	go wsConn.writePump()
}

// Send 向指定用户发送消息
func (s *WsConnServer) Send(userID string, msg *WsMsg) error {
	data, err := msg.ToJSON()
	if err != nil {
		return err
	}

	value, ok := s.conns.Load(userID)
	if !ok {
		return ErrUserNotOnline
	}

	wsConn := value.(*wsConnection)
	select {
	case wsConn.send <- data:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// SendBatch 批量向多个用户发送消息
func (s *WsConnServer) SendBatch(userIDs []string, msg *WsMsg) error {
	data, err := msg.ToJSON()
	if err != nil {
		return err
	}

	var lastErr error
	for _, userID := range userIDs {
		if err := s.SendRaw(userID, data); err != nil {
			logger.Warn("发送消息失败",
				zap.String("userID", userID),
				zap.Error(err))
			lastErr = err
		}
	}
	return lastErr
}

// SendRaw 发送原始字节数据
func (s *WsConnServer) SendRaw(userID string, data []byte) error {
	value, ok := s.conns.Load(userID)
	if !ok {
		return ErrUserNotOnline
	}

	wsConn := value.(*wsConnection)
	select {
	case wsConn.send <- data:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// Disconnect 断开指定用户的连接
func (s *WsConnServer) Disconnect(userID string) error {
	value, ok := s.conns.Load(userID)
	if !ok {
		return ErrUserNotOnline
	}

	wsConn := value.(*wsConnection)
	return wsConn.Close()
}

// IsOnline 检查用户是否在线
func (s *WsConnServer) IsOnline(userID string) bool {
	_, ok := s.conns.Load(userID)
	return ok
}

// OnMessage 注册消息接收回调
func (s *WsConnServer) OnMessage(fn func(userID string, msg *WsMsg)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.messageHandlers = append(s.messageHandlers, fn)
}

// OnConnect 注册用户连接回调
func (s *WsConnServer) OnConnect(fn func(userID string)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.connectHandlers = append(s.connectHandlers, fn)
}

// OnDisconnect 注册用户断开回调
func (s *WsConnServer) OnDisconnect(fn func(userID string)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.disconnectHandlers = append(s.disconnectHandlers, fn)
}

// Close 关闭连接
func (c *wsConnection) Close() error {
	var err error
	c.closeOnce.Do(func() {
		// 从连接池移除
		c.server.conns.Delete(c.userID)

		// 关闭WebSocket连接
		err = c.conn.Close()

		// 关闭发送队列
		close(c.send)

		// 触发断开回调
		c.server.handlersMu.RLock()
		handlers := c.server.disconnectHandlers
		c.server.handlersMu.RUnlock()
		for _, handler := range handlers {
			go handler(c.userID)
		}
	})
	return err
}

// readPump 读取消息循环
func (c *wsConnection) readPump() {
	defer c.Close()

	c.conn.SetReadLimit(512)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warn("WebSocket读取错误", zap.Error(err))
			}
			break
		}

		// 解析WebSocket消息
		wsMsg, err := ParseWsMsg(message)
		if err != nil {
			logger.Warn("解析消息失败", zap.String("userID", c.userID), zap.Error(err))
			continue
		}

		logger.Debug("收到消息",
			zap.String("userID", c.userID),
			zap.String("cmd", wsMsg.Cmd))

		// 触发消息回调
		c.server.handlersMu.RLock()
		handlers := c.server.messageHandlers
		c.server.handlersMu.RUnlock()
		for _, handler := range handlers {
			go handler(c.userID, wsMsg)
		}
	}
}

// writePump 写入消息循环
func (c *wsConnection) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// 发送队列已关闭
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				logger.Warn("WebSocket写入错误", zap.Error(err))
				return
			}

		case <-ticker.C:
			// 发送心跳
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
