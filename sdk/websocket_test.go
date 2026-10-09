package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
		if message.Cmd != "msg" {
			t.Errorf("cmd = %q, want msg", message.Cmd)
		}
		_ = conn.WriteJSON(WSMessage{
			Cmd:  "msg_reply",
			Seq:  message.Seq,
			Data: MessageReply{Status: "success", MessageID: 99},
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

	seq, err := connection.SendChat(1, 2, 0, "hello", "text")
	if err != nil {
		t.Fatal(err)
	}
	message, err := connection.Receive()
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
	if reply.Status != "success" || reply.MessageID != 99 {
		t.Fatalf("reply = %#v", reply)
	}
}
