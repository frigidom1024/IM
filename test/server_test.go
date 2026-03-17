package test

import (
	"gochat/pkg/logger"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func init() {
	// 初始化测试用的logger
	logger.Init("debug", "stdout")
}

// TestWsConnServer_Send 测试向单个用户发送消息
func TestWsConnServer_Send(t *testing.T) {
	server := NewWsConnServer()

	// 模拟用户连接
	userID := "user123"
	wsURL := createTestServer(server)
	defer httptest.NewServer(nil).Close()

	// 创建WebSocket连接
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer wsConn.Close()

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 测试发送消息
	msg := []byte("hello world")
	err = server.Send(userID, msg)
	if err != nil {
		t.Fatalf("Send失败: %v", err)
	}

	// 验证客户端收到消息
	_, recvMsg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}

	if string(recvMsg) != string(msg) {
		t.Errorf("消息不匹配，期望 %s，得到 %s", string(msg), string(recvMsg))
	}
}

// TestWsConnServer_SendBatch 测试批量发送消息
func TestWsConnServer_SendBatch(t *testing.T) {
	server := NewWsConnServer()

	userIDs := []string{"user1", "user2", "user3"}
	wsURL := createTestServer(server)

	// 创建多个连接
	conns := []*websocket.Conn{}
	for _, userID := range userIDs {
		wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
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
	time.Sleep(100 * time.Millisecond)

	// 批量发送消息
	msg := []byte("broadcast message")
	err := server.SendBatch(userIDs, msg)
	if err != nil {
		t.Fatalf("SendBatch失败: %v", err)
	}

	// 验证所有用户都收到消息
	for i, conn := range conns {
		_, recvMsg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("用户 %s 读取消息失败: %v", userIDs[i], err)
		}

		if string(recvMsg) != string(msg) {
			t.Errorf("用户 %s 消息不匹配", userIDs[i])
		}
	}
}

// TestWsConnServer_IsOnline 测试在线状态检查
func TestWsConnServer_IsOnline(t *testing.T) {
	server := NewWsConnServer()

	wsURL := createTestServer(server)

	userID := "user123"

	// 连接前应该离线
	if server.IsOnline(userID) {
		t.Error("用户未连接时IsOnline应该返回false")
	}

	// 创建连接
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 连接后应该在线
	if !server.IsOnline(userID) {
		t.Error("用户已连接时IsOnline应该返回true")
	}

	// 关闭连接
	wsConn.Close()
	time.Sleep(100 * time.Millisecond)

	// 断开后应该离线
	if server.IsOnline(userID) {
		t.Error("用户断开后IsOnline应该返回false")
	}
}

// TestWsConnServer_Disconnect 测试断开连接
func TestWsConnServer_Disconnect(t *testing.T) {
	server := NewWsConnServer()

	userID := "user123"
	wsURL := createTestServer(server)

	// 创建连接
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 主动断开
	err = server.Disconnect(userID)
	if err != nil {
		t.Fatalf("Disconnect失败: %v", err)
	}

	// 验证连接已断开
	time.Sleep(100 * time.Millisecond)
	if server.IsOnline(userID) {
		t.Error("Disconnect后用户应该离线")
	}

	// 尝试读取应该失败
	_, _, err = wsConn.ReadMessage()
	if err == nil {
		t.Error("连接应该已关闭")
	}
}

// TestWsConnServer_OnConnect 测试连接回调
func TestWsConnServer_OnConnect(t *testing.T) {
	server := NewWsConnServer()

	wsURL := createTestServer(server)

	var connectedUserID string
	server.OnConnect(func(userID string) {
		connectedUserID = userID
	})

	userID := "user123"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer wsConn.Close()

	// 等待回调
	time.Sleep(100 * time.Millisecond)

	if connectedUserID != userID {
		t.Errorf("OnConnect回调未正确触发，期望 %s，得到 %s", userID, connectedUserID)
	}
}

// TestWsConnServer_OnDisconnect 测试断开回调
func TestWsConnServer_OnDisconnect(t *testing.T) {
	server := NewWsConnServer()

	wsURL := createTestServer(server)

	var disconnectedUserID string
	server.OnDisconnect(func(userID string) {
		disconnectedUserID = userID
	})

	userID := "user123"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 关闭连接
	wsConn.Close()

	// 等待回调
	time.Sleep(200 * time.Millisecond)

	if disconnectedUserID != userID {
		t.Errorf("OnDisconnect回调未正确触发，期望 %s，得到 %s", userID, disconnectedUserID)
	}
}

// TestWsConnServer_OnMessage 测试消息回调
func TestWsConnServer_OnMessage(t *testing.T) {
	server := NewWsConnServer()

	wsURL := createTestServer(server)

	var receivedMsg atomic.Value
	server.OnMessage(func(userID string, msg []byte) {
		receivedMsg.Store(struct {
			userID string
			msg    []byte
		}{userID, msg})
	})

	userID := "user123"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer wsConn.Close()

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 发送消息
	msg := []byte("test message")
	err = wsConn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	// 等待回调
	time.Sleep(100 * time.Millisecond)

	data := receivedMsg.Load()
	if data == nil {
		t.Fatal("OnMessage回调未触发")
	}

	result := data.(struct {
		userID string
		msg    []byte
	})

	if result.userID != userID {
		t.Errorf("userID不匹配，期望 %s，得到 %s", userID, result.userID)
	}

	if string(result.msg) != string(msg) {
		t.Errorf("消息不匹配，期望 %s，得到 %s", string(msg), string(result.msg))
	}
}

// TestWsConnServer_MultipleHandlers 测试多个回调函数
func TestWsConnServer_MultipleHandlers(t *testing.T) {
	server := NewWsConnServer()

	wsURL := createTestServer(server)

	var count1, count2 atomic.Int32

	// 注册多个回调
	server.OnConnect(func(userID string) {
		count1.Add(1)
	})
	server.OnConnect(func(userID string) {
		count2.Add(1)
	})

	userID := "user123"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?userID="+userID, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer wsConn.Close()

	// 等待回调
	time.Sleep(100 * time.Millisecond)

	if count1.Load() != 1 {
		t.Errorf("回调1应该被调用1次，实际 %d", count1.Load())
	}

	if count2.Load() != 1 {
		t.Errorf("回调2应该被调用1次，实际 %d", count2.Load())
	}
}

// createTestServer 创建测试用的HTTP服务器，返回ws:// URL
func createTestServer(server *WsConnServer) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		server.HandleWebSocket(w, r)
	})

	testServer := httptest.NewServer(mux)
	// 将 http:// 转换为 ws://
	return "ws" + testServer.URL[len("http"):]
}
