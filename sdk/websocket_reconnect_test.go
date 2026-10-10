package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketReceiveReconnectsAfterDisconnect(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		if connections.Add(1) == 1 {
			_ = conn.Close()
			return
		}
		_ = conn.WriteJSON(WSMessage{Cmd: "ping"})
		select {}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, WithToken("token"))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	message, err := connection.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if message.Cmd != "ping" {
		t.Fatalf("message cmd = %q, want ping", message.Cmd)
	}
	if connections.Load() != 2 {
		t.Fatalf("connections = %d, want 2", connections.Load())
	}

	_ = time.Second // keep the test's timeout vocabulary explicit for future extensions
}
