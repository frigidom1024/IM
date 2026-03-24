package test

import (
	"context"
	"encoding/json"
	"gochat/connect"
	"gochat/pkg/logger"
	"gochat/pkg/snowflake"
	"gochat/pkg/util"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func init() {
	// 初始化测试用的logger
	logger.Init("debug", "stdout")
	// 初始化雪花ID生成器
	snowflake.Init(1, 1)
	// 初始化 JWT
	util.Init("test-secret")
}

// ========== 测试新的 Handler 系统 ==========

// TestHandlerRegistry 测试 HandlerRegistry
func TestHandlerRegistry(t *testing.T) {
	registry := connect.NewHandlerRegistry()

	// 测试设置消息处理器
	msgHandler := &mockMessageHandler{}
	registry.SetMessageHandler(msgHandler)

	ctx := context.Background()
	msg := &connect.WsMsg{Cmd: connect.CmdMsg}
	registry.HandleMessage(ctx, "user123", msg)

	if !msgHandler.called {
		t.Error("消息处理器应该被调用")
	}

	// 测试设置连接处理器
	connHandler := &mockConnectHandler{}
	registry.SetConnectHandler(connHandler)
	registry.HandleConnect(ctx, "user456")

	if !connHandler.called {
		t.Error("连接处理器应该被调用")
	}

	// 测试设置断开处理器
	disconnHandler := &mockDisconnectHandler{}
	registry.SetDisconnectHandler(disconnHandler)
	registry.HandleDisconnect(ctx, "user789")

	if !disconnHandler.called {
		t.Error("断开处理器应该被调用")
	}
}

// TestConnectServer_LifeCycle 测试连接服务器生命周期
func TestConnectServer_LifeCycle(t *testing.T) {
	server := connect.NewWsConnServer()
	registry := server.Registry()

	var connectCount, disconnectCount atomic.Int32

	// 设置连接处理器
	registry.SetConnectHandler(&mockConnectHandler{
		onConnect: func(ctx context.Context, userID string) error {
			connectCount.Add(1)
			return nil
		},
	})

	// 设置断开处理器
	registry.SetDisconnectHandler(&mockDisconnectHandler{
		onDisconnect: func(ctx context.Context, userID string) error {
			disconnectCount.Add(1)
			return nil
		},
	})

	// 追加消息处理器（处理ping并返回pong，不影响其他处理器）
	registry.RegisterMessageHandler(&mockMessageHandler{
		onMessage: func(userID string, msg *connect.WsMsg) error {
			// 处理ping消息，发送pong响应
			if msg.Cmd == connect.CmdPing {
				pongMsg := connect.NewPongMsg()
				return server.Send(userID, pongMsg)
			}
			return nil
		},
	})

	// 创建测试服务器
	wsURL := createTestServer(server)

	userID := "123"

	// 测试连接前状态
	if server.IsOnline(userID) {
		t.Error("用户未连接时应该离线")
	}

	// 创建连接
	token := createTestToken(t, userID)
	wsURL = wsURL + "/ws?token=" + token
	logger.Info("正在连接 WebSocket", zap.String("url", wsURL))
	wsConn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("连接失败: %v, 响应: %s", err, string(body))
		}
		t.Fatalf("连接失败: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}

	// 等待连接建立和回调
	time.Sleep(200 * time.Millisecond)

	// 验证连接状态
	if !server.IsOnline(userID) {
		t.Error("用户已连接应该在线")
	}

	// 验证连接回调
	if connectCount.Load() != 1 {
		t.Errorf("连接回调应该被调用1次，实际 %d", connectCount.Load())
	}

	// 测试发送心跳
	pingMsg := connect.NewWsMsg(connect.CmdPing, "", nil)
	pingData, _ := json.Marshal(pingMsg)
	err = wsConn.WriteMessage(websocket.TextMessage, pingData)
	if err != nil {
		t.Fatalf("发送心跳失败: %v", err)
	}

	// 读取 pong 响应
	_, pongData, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	var pongMsg connect.WsMsg
	if err := json.Unmarshal(pongData, &pongMsg); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if pongMsg.Cmd != connect.CmdPong {
		t.Errorf("期望收到 pong，实际收到 %s", pongMsg.Cmd)
	}

	// 关闭连接
	wsConn.Close()
	time.Sleep(200 * time.Millisecond)

	// 验证断开状态
	if server.IsOnline(userID) {
		t.Error("用户断开应该离线")
	}

	// 验证断开回调
	if disconnectCount.Load() != 1 {
		t.Errorf("断开回调应该被调用1次，实际 %d", disconnectCount.Load())
	}
}

// TestConnectServer_MultipleConnections 测试多连接
func TestConnectServer_MultipleConnections(t *testing.T) {
	server := connect.NewWsConnServer()
	registry := server.Registry()

	var connectCount atomic.Int32
	registry.SetConnectHandler(&mockConnectHandler{
		onConnect: func(ctx context.Context, userID string) error {
			connectCount.Add(1)
			return nil
		},
	})

	registry.RegisterMessageHandler(&mockMessageHandler{})

	wsURL := createTestServer(server)

	userIDs := []string{"1", "2", "3"}
	conns := []*websocket.Conn{}

	for _, userID := range userIDs {
		token := createTestToken(t, userID)
		wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+token, nil)
		if err != nil {
			t.Fatalf("连接失败 %s: %v", userID, err)
		}
		conns = append(conns, wsConn)
	}

	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	// 等待连接建立
	time.Sleep(200 * time.Millisecond)

	count := connectCount.Load()
	if count != 3 {
		t.Errorf("期望3个连接回调，实际 %d", count)
	}

	// 验证所有用户在线
	for _, userID := range userIDs {
		if !server.IsOnline(userID) {
			t.Errorf("用户 %s 应该在线", userID)
		}
	}
}

// TestConnectServer_SingleLogin 测试单点登录
func TestConnectServer_SingleLogin(t *testing.T) {
	server := connect.NewWsConnServer()
	registry := server.Registry()

	var connectCount atomic.Int32
	registry.SetConnectHandler(&mockConnectHandler{
		onConnect: func(ctx context.Context, userID string) error {
			connectCount.Add(1)
			return nil
		},
	})

	registry.SetDisconnectHandler(&mockDisconnectHandler{})

	registry.RegisterMessageHandler(&mockMessageHandler{})

	wsURL := createTestServer(server)

	userID := "123"

	// 创建第一个连接
	token := createTestToken(t, userID)
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+token, nil)
	if err != nil {
		t.Fatalf("第一次连接失败: %v", err)
	}
	defer conn1.Close()

	time.Sleep(100 * time.Millisecond)

	// 创建第二个连接（应该踢掉第一个）
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+token, nil)
	if err != nil {
		t.Fatalf("第二次连接失败: %v", err)
	}
	defer conn2.Close()

	time.Sleep(200 * time.Millisecond)

	// 连接回调应该被调用2次
	secondCount := connectCount.Load()
	if secondCount != 2 {
		t.Errorf("期望2次连接回调，实际 %d", secondCount)
	}

	// 第一个连接应该已断开
	_, _, err = conn1.ReadMessage()
	if err == nil {
		t.Error("第一个连接应该被关闭")
	}
}

// ========== Mock 类型 ==========

type mockMessageHandler struct {
	called    bool
	onMessage func(userID string, msg *connect.WsMsg) error
}

func (m *mockMessageHandler) Handle(ctx context.Context, userID string, msg *connect.WsMsg) error {
	m.called = true
	if m.onMessage != nil {
		return m.onMessage(userID, msg)
	}
	return nil
}

type mockConnectHandler struct {
	called    bool
	onConnect func(ctx context.Context, userID string) error
}

func (m *mockConnectHandler) OnConnect(ctx context.Context, userID string) error {
	m.called = true
	if m.onConnect != nil {
		return m.onConnect(ctx, userID)
	}
	return nil
}

type mockDisconnectHandler struct {
	called       bool
	onDisconnect func(ctx context.Context, userID string) error
}

func (m *mockDisconnectHandler) OnDisconnect(ctx context.Context, userID string) error {
	m.called = true
	if m.onDisconnect != nil {
		return m.onDisconnect(ctx, userID)
	}
	return nil
}

// ========== 辅助函数 ==========

// createTestServer 创建测试服务器
func createTestServer(server *connect.WsConnServer) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("收到 WebSocket 连接请求", zap.String("path", r.URL.Path), zap.String("query", r.URL.RawQuery))
		server.HandleWebSocket(w, r)
	})

	testServer := httptest.NewServer(mux)
	// 将 http:// 转换为 ws://
	wsURL := "ws" + testServer.URL[len("http"):]
	logger.Info("测试服务器启动", zap.String("ws_url", wsURL), zap.String("http_url", testServer.URL))
	return wsURL
}

// createTestToken 创建测试用的 JWT token
func createTestToken(t *testing.T, userID string) string {
	// 将 userID 字符串转换为整数
	userIDInt := 0
	for _, c := range userID {
		userIDInt = userIDInt*10 + int(c-'0')
	}

	// 生成 token
	token, err := util.GenerateToken(userIDInt, "testuser", 24)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	// 验证 token 是否能正确解析
	_, err = util.ParseToken(token)
	if err != nil {
		t.Fatalf("生成的 token 无法解析: %v", err)
	}

	return token
}
