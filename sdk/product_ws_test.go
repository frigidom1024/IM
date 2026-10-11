package sdk_test

import (
	"context"
	"github.com/gorilla/websocket"
	"gochat/sdk"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReconnectUsesRefreshedToken(t *testing.T) {
	var tokens atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if token == "Bearer fresh-1" {
			return
		}
		if token != "Bearer fresh-2" {
			t.Errorf("reconnect token=%s", token)
			return
		}
		conn.WriteJSON(sdk.WSMessage{Cmd: "ping"})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("initial"), sdk.WithTokenRefresh(func(context.Context, string) (string, error) {
		if tokens.Add(1) == 1 {
			return "fresh-1", nil
		}
		return "fresh-2", nil
	}))
	ws, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	msg, err := ws.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Cmd != "ping" {
		t.Fatal(msg)
	}
}
func TestConcurrentWebSocketSends(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for received.Load() < 40 {
			var msg sdk.WSMessage
			if conn.ReadJSON(&msg) != nil {
				return
			}
			received.Add(1)
		}
		conn.WriteJSON(sdk.WSMessage{Cmd: "done"})
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	ws, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ws.Ping(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	msg, err := ws.Receive()
	if err != nil || msg.Cmd != "done" {
		t.Fatalf("message=%v error=%v", msg, err)
	}
}

func TestCloseCancelsReconnect(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	var calls atomic.Int32
	entered := make(chan struct{})
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"), sdk.WithTokenRefresh(func(ctx context.Context, current string) (string, error) {
		if calls.Add(1) == 1 {
			return current, nil
		}
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}))
	ws, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ws.Reconnect(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reconnect not started")
	}
	ws.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not cancel reconnect")
	}
}
