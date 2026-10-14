package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type heartbeatEvent struct {
	connection int32
	message    WSMessage
}

func heartbeatTestClient(t *testing.T, interval time.Duration) (*WebSocketClient, <-chan heartbeatEvent) {
	t.Helper()
	events := make(chan heartbeatEvent, 256)
	var connections atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		id := connections.Add(1)
		for {
			var message WSMessage
			if err := conn.ReadJSON(&message); err != nil {
				return
			}
			events <- heartbeatEvent{connection: id, message: message}
			if message.Cmd == "ping" {
				if err := conn.WriteJSON(WSMessage{Cmd: "pong"}); err != nil {
					return
				}
			}
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, WithToken("token"), WithWebSocketHeartbeatInterval(interval))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws, events
}

func nextHeartbeatEvent(t *testing.T, events <-chan heartbeatEvent) heartbeatEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket message")
		return heartbeatEvent{}
	}
}

func TestWebSocketAutomaticHeartbeat(t *testing.T) {
	ws, events := heartbeatTestClient(t, 10*time.Millisecond)
	// No Send, Ping or Receive call is needed to start heartbeats.
	for i := 0; i < 2; i++ {
		event := nextHeartbeatEvent(t, events)
		if event.message.Cmd != "ping" || event.connection != 1 {
			t.Fatalf("heartbeat = %#v", event)
		}
	}
	message, err := receiveTestMessage(t, ws)
	if err != nil || message.Cmd != "pong" {
		t.Fatalf("heartbeat response = %#v, error = %v", message, err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ws.heartbeatDone:
	default:
		t.Fatal("Close returned before heartbeat stopped")
	}
	if err := ws.Ping(); err == nil {
		t.Fatal("Ping succeeded after Close")
	}
	if err := ws.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestWebSocketHeartbeatDisabled(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			ws, events := heartbeatTestClient(t, interval)
			select {
			case event := <-events:
				t.Fatalf("unexpected automatic heartbeat: %#v", event)
			case <-time.After(50 * time.Millisecond):
			}
			if err := ws.Ping(); err != nil {
				t.Fatal(err)
			}
			if event := nextHeartbeatEvent(t, events); event.message.Cmd != "ping" {
				t.Fatalf("manual ping = %#v", event)
			}
		})
	}
}

func TestWebSocketHeartbeatContinuesAfterReconnect(t *testing.T) {
	ws, events := heartbeatTestClient(t, 10*time.Millisecond)
	if event := nextHeartbeatEvent(t, events); event.connection != 1 {
		t.Fatalf("first heartbeat = %#v", event)
	}
	if err := ws.Reconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	for {
		event := nextHeartbeatEvent(t, events)
		if event.connection == 1 {
			continue // A heartbeat may already have been queued before reconnect.
		}
		if event.connection != 2 || event.message.Cmd != "ping" {
			t.Fatalf("heartbeat after reconnect = %#v", event)
		}
		break
	}
}

func TestWebSocketHeartbeatRecoversFailedWrite(t *testing.T) {
	ws, events := heartbeatTestClient(t, 10*time.Millisecond)
	previous, err := ws.currentConnection()
	if err != nil {
		t.Fatal(err)
	}
	// Force a write failure without a Receive call driving recovery.
	_ = previous.Close()
	for {
		event := nextHeartbeatEvent(t, events)
		if event.connection == 1 {
			continue
		}
		if event.connection != 2 || event.message.Cmd != "ping" {
			t.Fatalf("heartbeat after failed write = %#v", event)
		}
		break
	}
}

func TestWebSocketHeartbeatConcurrentWithSend(t *testing.T) {
	ws, events := heartbeatTestClient(t, time.Millisecond)
	nextHeartbeatEvent(t, events)
	var writers sync.WaitGroup
	for i := 0; i < 40; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			if _, err := ws.SendChat("private:2", "hello", "text"); err != nil {
				t.Error(err)
			}
		}()
	}
	writers.Wait()
	for received := 0; received < 40; {
		event := nextHeartbeatEvent(t, events)
		if event.connection != 1 {
			t.Fatalf("unexpected reconnect during concurrent writes: %#v", event)
		}
		switch event.message.Cmd {
		case "msg":
			received++
		case "ping":
		default:
			t.Fatalf("unexpected message: %#v", event)
		}
	}
}

func TestWebSocketConcurrentRecoveryReusesConnection(t *testing.T) {
	ws, events := heartbeatTestClient(t, 0)
	previous, err := ws.currentConnection()
	if err != nil {
		t.Fatal(err)
	}
	_ = previous.Close()
	var reconnects sync.WaitGroup
	for i := 0; i < 20; i++ {
		reconnects.Add(1)
		go func() {
			defer reconnects.Done()
			if err := ws.reconnectFrom(previous); err != nil {
				t.Error(err)
			}
		}()
	}
	reconnects.Wait()
	if err := ws.Ping(); err != nil {
		t.Fatal(err)
	}
	if event := nextHeartbeatEvent(t, events); event.connection != 2 {
		t.Fatalf("concurrent recovery created extra connections: %#v", event)
	}
}
