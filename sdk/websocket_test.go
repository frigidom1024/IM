package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConnectWebSocketAndSendChat(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		var message WSMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Errorf("read websocket message: %v", err)
			return
		}
		payload, _ := message.Data.(map[string]any)
		if payload["conversation_id"] != "private:2" || len(payload) != 3 {
			t.Errorf("chat payload=%#v", payload)
		}
		if message.Cmd != "msg" {
			t.Errorf("cmd = %q, want msg", message.Cmd)
		}
		_ = conn.WriteJSON(WSMessage{
			Cmd:  "msg_reply",
			Seq:  message.Seq,
			Data: MessageReply{Status: "success", ConversationID: "private:2", MessageID: 99, MessageIDString: "99"},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, WithToken("gochat-token"))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := client.ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	seq, err := connection.SendChat("private:2", "hello", "text")
	if err != nil {
		t.Fatal(err)
	}
	message, err := receiveTestMessage(t, connection)
	if err != nil {
		t.Fatal(err)
	}
	if message.Cmd != "msg_reply" || message.Seq != seq {
		t.Fatalf("message = %#v", message)
	}
	var reply MessageReply
	if err := message.DecodeData(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.Status != "success" || reply.MessageID != 99 || reply.ConversationID != "private:2" {
		t.Fatalf("reply = %#v", reply)
	}
}

func TestScopeValidationAndExactDownstreamIDs(t *testing.T) {
	for _, scope := range []string{"private:0", "private:01", "group:-1", "group:9223372036854775808", "other:1", "1"} {
		var client *WebSocketClient
		if _, err := client.SendChat(scope, "hello", "text"); err == nil {
			t.Fatal(scope)
		}
	}
	var chat ChatMessage
	raw := json.RawMessage(`{"id":"9007199254740993","conversation_id":"group:10","sender_id":"9223372036854775807","type":"text","content":"hello"}`)
	if err := (WSMessage{Data: raw}).DecodeData(&chat); err != nil {
		t.Fatal(err)
	}
	if chat.ID != 9007199254740993 || chat.SenderID != 9223372036854775807 || chat.ConversationID != "group:10" {
		t.Fatal(chat)
	}
	// Receive decodes generic JSON numbers as float64; the string ID is authoritative.
	var ack MessageReply
	if err := (WSMessage{Data: map[string]any{"message_id": float64(9223372036854775807), "message_id_string": "9223372036854775807", "conversation_id": "private:2"}}).DecodeData(&ack); err != nil {
		t.Fatal(err)
	}
	if ack.MessageID != 9223372036854775807 {
		t.Fatal(ack)
	}
}

func receiveTestMessage(t *testing.T, ws *WebSocketClient) (WSMessage, error) {
	t.Helper()
	select {
	case message, ok := <-ws.Receive():
		if !ok {
			return WSMessage{}, fmt.Errorf("message stream closed")
		}
		return message, nil
	case <-time.After(2 * time.Second):
		t.Fatal("message stream timed out")
		return WSMessage{}, nil
	}
}
