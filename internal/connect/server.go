package connect

import (
	"context"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/util"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// wsConnection WebSocket连接
type wsConnection struct {
	conn      *websocket.Conn
	userID    string
	send      chan []byte
	server    *WsConnServer
	closeOnce sync.Once

	// 速率限制
	lastMsgTime time.Time
	msgCount    int
}

func (c *wsConnection) rateLimited() bool {
	now := time.Now()
	if now.Sub(c.lastMsgTime) > time.Second {
		c.msgCount = 0
		c.lastMsgTime = now
	}
	c.msgCount++
	return c.msgCount > 30 // 每秒最多30条消息
}

// WsConnServer WebSocket连接服务器实现
type WsConnServer struct {
	conns      sync.Map // string(userID) -> *wsConnection
	registry   *HandlerRegistry
	serverMu   sync.Mutex
	httpServer *http.Server
}

// NewWsConnServer 创建新的WebSocket服务器
func NewWsConnServer() *WsConnServer {
	return &WsConnServer{
		registry: NewHandlerRegistry(),
	}
}

// Registry 获取处理器注册表
func (s *WsConnServer) Registry() *HandlerRegistry {
	return s.registry
}

// HandleWebSocket 处理WebSocket连接请求
func (s *WsConnServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := normalizeWebSocketToken(r.Header.Get("Authorization"))
	if token == "" {
		token = normalizeWebSocketToken(r.URL.Query().Get("token"))
	}
	if token == "" {
		logger.Warn("WebSocket连接失败: 缺少token")
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	claims, err := util.ParseToken(token)
	if err != nil {
		logger.Warn("Token验证失败", zap.Error(err))
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	userID := strconv.Itoa(claims.UserID)

	if existing, exists := s.conns.Load(userID); exists {
		logger.Info("踢出旧连接", zap.String("userID", userID))
		existing.(*wsConnection).Close()
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn("WebSocket升级失败", zap.String("userID", userID), zap.Error(err))
		return
	}

	wsConn := &wsConnection{
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, 64),
		server: s,
	}

	s.conns.Store(userID, wsConn)
	logger.Info("用户连接成功", zap.String("userID", userID))

	ctx := context.Background()
	s.registry.HandleConnect(ctx, userID)

	go wsConn.readPump()
	go wsConn.writePump()
}

func normalizeWebSocketToken(value string) string {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	if len(parts) == 1 && !strings.EqualFold(parts[0], "Bearer") {
		return parts[0]
	}
	return ""
}

// Send 向指定用户发送消息（Message 接口）
func (s *WsConnServer) Send(userID string, msg Message) error {
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
func (s *WsConnServer) SendBatch(userIDs []string, msg Message) error {
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

// Run 启动WebSocket服务器
func (s *WsConnServer) Run(addr string) error {
	return s.RunReady(context.Background(), addr, nil)
}

// RunReady binds the listener before reporting readiness.
func (s *WsConnServer) RunReady(ctx context.Context, addr string, ready chan<- struct{}) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.HandleWebSocket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: addr, Handler: mux}
	s.serverMu.Lock()
	s.httpServer = srv
	s.serverMu.Unlock()
	if ready != nil {
		close(ready)
	}
	logger.Info("启动WebSocket服务器", zap.String("addr", addr))
	return srv.Serve(listener)
}

// Shutdown closes active connections and stops the WebSocket listener.
func (s *WsConnServer) Shutdown(ctx context.Context) error {
	s.conns.Range(func(_, value any) bool {
		_ = value.(*wsConnection).Close()
		return true
	})
	s.serverMu.Lock()
	server := s.httpServer
	s.serverMu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

// RunWithServeMux 使用已有的ServeMux运行
func (s *WsConnServer) RunWithServeMux(mux *http.ServeMux, prefix string) {
	mux.HandleFunc(prefix+"/ws", s.HandleWebSocket)
}

// SendOfflineNotify 发送离线通知给用户
func (s *WsConnServer) SendOfflineNotify(userID string, notifyData *OfflineNotifyData) error {
	notifyMsg := NewRawMessage(CmdOfflineNotify, "", notifyData)
	return s.Send(userID, notifyMsg)
}

// Close 关闭连接
func (c *wsConnection) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.server.conns.Delete(c.userID)
		// Close may run from readPump while writePump is sending data.
		_ = c.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
		err = c.conn.Close()
		close(c.send)

		ctx := context.Background()
		c.server.registry.HandleDisconnect(ctx, c.userID)
	})
	return err
}

// readPump 读取消息循环
func (c *wsConnection) readPump() {
	defer c.Close()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logger.Warn("WebSocket异常关闭",
					zap.String("userID", c.userID),
					zap.Error(err))
			} else {
				logger.Debug("WebSocket连接关闭",
					zap.String("userID", c.userID),
					zap.Error(err))
			}
			break
		}

		if c.rateLimited() {
			logger.Warn("消息频率过高，丢弃", zap.String("userID", c.userID))
			continue
		}

		wsMsg, err := ParseWsMsg(message)
		if err != nil {
			logger.Warn("解析消息失败",
				zap.String("userID", c.userID),
				zap.String("raw", truncateString(string(message), 200)),
				zap.Error(err))
			continue
		}

		logger.Debug("收到消息",
			zap.String("userID", c.userID),
			zap.String("cmd", wsMsg.Cmd()))

		ctx := context.Background()
		c.server.registry.HandleMessage(ctx, c.userID, wsMsg)
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
				// Control writes are safe alongside writePump's single data writer.
				c.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				logger.Warn("WebSocket写入错误",
					zap.String("userID", c.userID),
					zap.Error(err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Warn("WebSocket心跳发送失败",
					zap.String("userID", c.userID),
					zap.Error(err))
				return
			}
		}
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
