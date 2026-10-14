package application

import (
	"context"
	"github.com/gorilla/websocket"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/util"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConnectDoesNotReadDatabaseOrPushNotifications(t *testing.T) {
	if err := logger.Init("error", "stdout"); err != nil {
		t.Fatal(err)
	}
	util.Init("no-offline-notification-test")
	token, err := util.GenerateToken(1, "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	// No message/group service or database is required to establish a connection.
	ws := newWebSocketServer(nil, nil)
	server := httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	defer server.Close()
	defer ws.Shutdown(context.Background())
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err = conn.WriteJSON(map[string]any{"cmd": "ping"}); err != nil {
		t.Fatal(err)
	}
	var reply struct{ Cmd string }
	if err = conn.ReadJSON(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.Cmd != "pong" {
		t.Fatalf("unexpected connect push: %s", reply.Cmd)
	}
}
