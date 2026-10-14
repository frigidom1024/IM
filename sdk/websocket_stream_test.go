package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func streamTestClient(t *testing.T, handler http.HandlerFunc) *WebSocketClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, WithToken("token"), WithWebSocketHeartbeatInterval(0))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}
func waitStreamClosed(t *testing.T, stream <-chan WSMessage) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-stream:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("receive stream did not close")
		}
	}
}

func TestReceiveStreamKeepsOneConnectionAndDeliversInOrder(t *testing.T) {
	var connections atomic.Int32
	ws := streamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connections.Add(1)
		for i := 0; i < 8; i++ {
			if err = conn.WriteJSON(WSMessage{Cmd: "msg", Seq: fmt.Sprint(i)}); err != nil {
				return
			}
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	stream := ws.Receive()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ws.Receive() != stream {
				t.Error("Receive started another stream")
			}
		}()
	}
	wg.Wait()
	for i := 0; i < 8; i++ {
		message, err := receiveTestMessage(t, ws)
		if err != nil || message.Seq != fmt.Sprint(i) {
			t.Fatal(message, err)
		}
	}
	if connections.Load() != 1 {
		t.Fatal("receiving opened additional sockets", connections.Load())
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	waitStreamClosed(t, stream)
	if _, ok := <-ws.Errors(); ok {
		t.Fatal("clean close reported an error")
	}
}

func TestMalformedJSONReportsErrorsWithoutReconnecting(t *testing.T) {
	var connections atomic.Int32
	ws := streamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connections.Add(1)
		// Overflow the diagnostic buffer while its consumer is deliberately idle.
		for i := 1; i <= 32; i++ {
			if err = conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("{", i))); err != nil {
				return
			}
		}
		if err = conn.WriteJSON(WSMessage{Cmd: "msg", Seq: "after malformed frames"}); err != nil {
			return
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	message, err := receiveTestMessage(t, ws)
	if err != nil || message.Seq != "after malformed frames" {
		t.Fatal(message, err)
	}
	if connections.Load() != 1 {
		t.Fatal("JSON decoding reconnected the socket")
	}
	ws.Close()
	count := 0
	var last error
	for err := range ws.Errors() {
		count++
		last = err
		var syntax *json.SyntaxError
		if !errors.As(err, &syntax) {
			t.Fatal("decode cause was lost", err)
		}
	}
	if count != 16 || last == nil {
		t.Fatal("diagnostic stream failed to preserve recent errors", count, last)
	}
}

func TestCloseUnblocksFullMessageStream(t *testing.T) {
	ws := streamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for i := 0; i < 128; i++ {
			if err = conn.WriteJSON(WSMessage{Cmd: "msg", Seq: fmt.Sprint(i)}); err != nil {
				return
			}
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	stream := ws.Receive()
	deadline := time.Now().Add(2 * time.Second)
	for len(stream) < cap(stream) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(stream) != cap(stream) {
		t.Fatal("did not fill the stream")
	}
	done := make(chan error, 1)
	go func() { done <- ws.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close blocked on an idle consumer")
	}
	waitStreamClosed(t, stream)
}

func TestRepeatedDisconnectsTerminateWithErrorAndCloseStreams(t *testing.T) {
	var connections atomic.Int32
	ws := streamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connections.Add(1)
		conn.Close()
	})
	waitStreamClosed(t, ws.Receive())
	var terminal error
	for err := range ws.Errors() {
		terminal = err
	}
	if terminal == nil || !strings.Contains(terminal.Error(), "repeatedly disconnected") {
		t.Fatal("missing terminal read error", terminal)
	}
	if connections.Load() != 5 {
		t.Fatal("unbounded reconnect loop", connections.Load())
	}
	if err := ws.Ping(); err == nil {
		t.Fatal("terminated client still sends")
	}
	ws.Close()
}

func TestReconnectFailureRetainsReadCauseAndStopsStream(t *testing.T) {
	var attempts atomic.Int32
	ws := streamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) > 1 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			conn.Close()
		}
	})
	waitStreamClosed(t, ws.Receive())
	var terminal error
	for err := range ws.Errors() {
		terminal = err
	}
	var closed *websocket.CloseError
	if terminal == nil || !strings.Contains(terminal.Error(), "reconnect") || !errors.As(terminal, &closed) {
		t.Fatal("lost original read failure", terminal)
	}
	if attempts.Load() != 6 {
		t.Fatal("unexpected reconnect attempt count", attempts.Load())
	}
}

func TestNilReceiveStreamIsClosed(t *testing.T) {
	var ws *WebSocketClient
	if _, ok := <-ws.Receive(); ok {
		t.Fatal("nil receive returned a message")
	}
	if err, ok := <-ws.Errors(); !ok || err == nil {
		t.Fatal("nil client error missing")
	}
}
