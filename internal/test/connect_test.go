//go:build integration

package test

import (
	"context"
	"encoding/json"
	"gochat/internal/connect"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/snowflake"
	"gochat/internal/pkg/util"
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
	logger.Init("debug", "stdout")
	snowflake.Init(1, 1)
	util.Init("test-secret")
}

// TestHandlerRegistry 测试 HandlerRegistry
func TestHandlerRegistry(t *testing.T) {
	registry := connect.NewHandlerRegistry()

	msgHandler := &mockMessageHandler{}
	registry.Register(connect.CmdMsg, msgHandler)

	ctx := context.Background()
	msg := connect.NewRawMessage(connect.CmdMsg, "", nil)
	registry.HandleMessage(ctx, "user123", msg)

	if !msgHandler.called {
		t.Error("消息处理器应该被调用")
	}

	connHandler := &mockConnectHandler{}
	registry.RegisterConnectHandler(connHandler)
	registry.HandleConnect(ctx, "user456")

	if !connHandler.called {
		t.Error("连接处理器应该被调用")
	}

	disconnHandler := &mockDisconnectHandler{}
	registry.RegisterDisconnectHandler(disconnHandler)
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

	registry.RegisterConnectHandler(&mockConnectHandler{
		onConnect: func(ctx context.Context, userID string) error {
			connectCount.Add(1)
			return nil
		},
	})

	registry.RegisterDisconnectHandler(&mockDisconnectHandler{
		onDisconnect: func(ctx context.Context, userID string) error {
			disconnectCount.Add(1)
			return nil
		},
	})

	registry.Register(connect.CmdPing, &mockMessageHandler{
		onMessage: func(userID string, msg connect.Message) error {
			if msg.Cmd() == connect.CmdPing {
				pongMsg := connect.NewPongMsg()
				return server.Send(userID, pongMsg)
			}
			return nil
		},
	})

	wsURL := createTestServer(server)
	userID := "123"

	if server.IsOnline(userID) {
		t.Error("用户未连接时应该离线")
	}

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

	time.Sleep(200 * time.Millisecond)

	if !server.IsOnline(userID) {
		t.Error("用户已连接应该在线")
	}

	if connectCount.Load() != 1 {
		t.Errorf("连接回调应该被调用1次，实际 %d", connectCount.Load())
	}

	// 发送心跳
	pingMsg := connect.NewRawMessage(connect.CmdPing, "", nil)
	pingData, _ := json.Marshal(pingMsg)
	err = wsConn.WriteMessage(websocket.TextMessage, pingData)
	if err != nil {
		t.Fatalf("发送心跳失败: %v", err)
	}

	_, pongData, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	var pongMsg connect.RawMessage
	if err := json.Unmarshal(pongData, &pongMsg); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if pongMsg.Cmd() != connect.CmdPong {
		t.Errorf("期望收到 pong，实际收到 %s", pongMsg.Cmd())
	}

	wsConn.Close()
	time.Sleep(200 * time.Millisecond)

	if server.IsOnline(userID) {
		t.Error("用户断开应该离线")
	}

	if disconnectCount.Load() != 1 {
		t.Errorf("断开回调应该被调用1次，实际 %d", disconnectCount.Load())
	}
}

// TestConnectServer_MultipleConnections 测试多连接
func TestConnectServer_MultipleConnections(t *testing.T) {
	server := connect.NewWsConnServer()
	registry := server.Registry()

	var connectCount atomic.Int32
	registry.RegisterConnectHandler(&mockConnectHandler{
		onConnect: func(ctx context.Context, userID string) error {
			connectCount.Add(1)
			return nil
		},
	})

	registry.Register(connect.CmdPing, &mockMessageHandler{})

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

	time.Sleep(200 * time.Millisecond)

	count := connectCount.Load()
	if count != 3 {
		t.Errorf("期望3个连接回调，实际 %d", count)
	}

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
	registry.RegisterConnectHandler(&mockConnectHandler{
		onConnect: func(ctx context.Context, userID string) error {
			connectCount.Add(1)
			return nil
		},
	})

	registry.RegisterDisconnectHandler(&mockDisconnectHandler{})
	registry.Register(connect.CmdPing, &mockMessageHandler{})

	wsURL := createTestServer(server)
	userID := "123"

	token := createTestToken(t, userID)
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+token, nil)
	if err != nil {
		t.Fatalf("第一次连接失败: %v", err)
	}
	defer conn1.Close()

	time.Sleep(100 * time.Millisecond)

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+token, nil)
	if err != nil {
		t.Fatalf("第二次连接失败: %v", err)
	}
	defer conn2.Close()

	time.Sleep(200 * time.Millisecond)

	secondCount := connectCount.Load()
	if secondCount != 2 {
		t.Errorf("期望2次连接回调，实际 %d", secondCount)
	}

	_, _, err = conn1.ReadMessage()
	if err == nil {
		t.Error("第一个连接应该被关闭")
	}
}

// ========== Mock 类型 ==========

type mockMessageHandler struct {
	called    bool
	onMessage func(userID string, msg connect.Message) error
}

func (m *mockMessageHandler) Handle(ctx context.Context, userID string, msg connect.Message) error {
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

func createTestServer(server *connect.WsConnServer) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("收到 WebSocket 连接请求", zap.String("path", r.URL.Path), zap.String("query", r.URL.RawQuery))
		server.HandleWebSocket(w, r)
	})

	testServer := httptest.NewServer(mux)
	wsURL := "ws" + testServer.URL[len("http"):]
	logger.Info("测试服务器启动", zap.String("ws_url", wsURL), zap.String("http_url", testServer.URL))
	return wsURL
}

func createTestToken(t *testing.T, userID string) string {
	userIDInt := 0
	for _, c := range userID {
		userIDInt = userIDInt*10 + int(c-'0')
	}

	token, err := util.GenerateToken(userIDInt, "testuser", 24)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	_, err = util.ParseToken(token)
	if err != nil {
		t.Fatalf("生成的 token 无法解析: %v", err)
	}

	return token
}
