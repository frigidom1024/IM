package api

import (
	"context"
	"encoding/json"
	"fmt"
	"gochat/internal/connect"
	wsHandler "gochat/internal/connect/handler"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/snowflake"
	"gochat/sdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConversationProtocolAndActiveNotificationPull(t *testing.T) {
	f := newRelationshipFixture(t) // Only runs with an explicitly supplied isolated DB.
	a, b := f.users[0].ID, f.users[1].ID
	f.call(0, "POST", "/api/friend/add", map[string]any{"friend_id": b}, 200)
	f.call(1, "POST", "/api/friend/add", map[string]any{"friend_id": a}, 200)
	if err := snowflake.Init(0, 1); err != nil {
		t.Fatal(err)
	}
	messaging := service.NewMessagingService(registry.NewMessageRepository(), registry.NewConversationRepository())
	if _, err := messaging.Send(&model.Message{ID: snowflake.MustNextID(), FromID: a, ToID: b, MsgType: "text", Content: "before connect"}); err != nil {
		t.Fatal(err)
	}
	ws := connect.NewWsConnServer()
	ws.Registry().Register(connect.CmdMsg, wsHandler.NewChatHandler(messaging, service.NewGroupService(registry.NewGroupRepository(), registry.NewGroupMemberRepository()), ws))
	ws.Registry().Register(connect.CmdPing, wsHandler.NewPingHandler(ws))
	ws.Registry().RegisterConnectHandler(wsHandler.NewDefaultConnectHandler())
	server := httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	defer server.Close()
	defer ws.Shutdown(context.Background())
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	clients := make([]*sdk.Client, 3)
	sockets := make([]*sdk.WebSocketClient, 3)
	for i := range clients {
		var err error
		clients[i], err = sdk.NewClient(f.server.URL, sdk.WithToken(f.tokens[i]), sdk.WithWebSocketURL(endpoint), sdk.WithWebSocketHeartbeatInterval(0))
		if err != nil {
			t.Fatal(err)
		}
		sockets[i], err = clients[i].ConnectWebSocket(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer sockets[i].Close()
		timer := time.AfterFunc(10*time.Second, func() { sockets[i].Close() })
		defer timer.Stop()
		if err = sockets[i].Ping(); err != nil {
			t.Fatal(err)
		}
		reply, err := receiveProtocolMessage(t, sockets[i])
		if err != nil {
			t.Fatal(err)
		}
		if reply.Cmd != "pong" {
			t.Fatalf("connection pushed a notification: %s", reply.Cmd)
		}
	}
	unread, err := clients[1].GetUnreadSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if unread.Count != 1 || len(unread.UnreadList) != 1 || unread.UnreadList[0].ID != fmt.Sprintf("private:%d", a) {
		t.Fatal(unread)
	}
	scope := fmt.Sprintf("private:%d", b)
	seq, err := sockets[0].SendChat(scope, "live", "text")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := receiveProtocolMessage(t, sockets[0])
	if err != nil {
		t.Fatal(err)
	}
	var ack sdk.MessageReply
	if frame.Cmd != "msg_reply" || frame.Seq != seq || frame.DecodeData(&ack) != nil || ack.ConversationID != scope || ack.Duplicate {
		t.Fatal(frame, ack)
	}
	delivery, err := receiveProtocolMessage(t, sockets[1])
	if err != nil {
		t.Fatal(err)
	}
	var message sdk.ChatMessage
	if delivery.Cmd != "msg" || delivery.Seq != seq || delivery.DecodeData(&message) != nil || message.ConversationID != fmt.Sprintf("private:%d", a) || message.SenderID != a || message.ID != ack.MessageID {
		t.Fatal(delivery, message)
	}
	for _, field := range []string{"from_id", "to_id", "group_id"} {
		if _, ok := delivery.Data.(map[string]any)[field]; ok {
			t.Fatal("obsolete routing field", field)
		}
	}
	if err = sockets[0].Send(sdk.WSMessage{Cmd: "msg", Seq: seq, Data: sdk.ChatMessage{ConversationID: scope, Type: "text", Content: "live"}}); err != nil {
		t.Fatal(err)
	}
	frame, err = receiveProtocolMessage(t, sockets[0])
	if err != nil {
		t.Fatal(err)
	}
	var retry sdk.MessageReply
	if frame.DecodeData(&retry) != nil || !retry.Duplicate || retry.MessageID != ack.MessageID || retry.ConversationID != scope {
		t.Fatal(frame)
	}
	unread, err = clients[1].GetUnreadSummary(context.Background())
	if err != nil || unread.Count != 2 {
		t.Fatal(unread, err)
	}
	if err = clients[1].MarkConversationRead(context.Background(), message.ConversationID); err != nil {
		t.Fatal(err)
	}
	unread, err = clients[1].GetUnreadSummary(context.Background())
	if err != nil || unread.Count != 0 {
		t.Fatal(unread, err)
	}
	var group model.Group
	if err = json.Unmarshal(f.call(0, "POST", "/api/group/create", map[string]any{"name": "ws-scope-team", "max_members": 3}, 200), &group); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{b, f.users[2].ID} {
		f.call(0, "POST", fmt.Sprintf("/api/group/%d/invite", group.ID), map[string]any{"user_id": id}, 200)
	}
	groupScope := fmt.Sprintf("group:%d", group.ID)
	seq, err = sockets[0].SendChat(groupScope, "group live", "text")
	if err != nil {
		t.Fatal(err)
	}
	frame, err = receiveProtocolMessage(t, sockets[0])
	if err != nil || frame.DecodeData(&ack) != nil || ack.ConversationID != groupScope {
		t.Fatal(frame, err)
	}
	for _, socket := range sockets[1:] {
		frame, err = receiveProtocolMessage(t, socket)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Seq != seq || frame.DecodeData(&message) != nil || message.ConversationID != groupScope || message.SenderID != a {
			t.Fatal(frame, message)
		}
	}
	if err = sockets[1].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = sockets[0].SendChat(scope, "while offline", "text"); err != nil {
		t.Fatal(err)
	}
	frame, err = receiveProtocolMessage(t, sockets[0])
	if err != nil || frame.Cmd != "msg_reply" {
		t.Fatal(frame, err)
	}
	reconnected, err := clients[1].ConnectWebSocket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	timer := time.AfterFunc(10*time.Second, func() { reconnected.Close() })
	defer timer.Stop()
	if err = reconnected.Ping(); err != nil {
		t.Fatal(err)
	}
	frame, err = receiveProtocolMessage(t, reconnected)
	if err != nil || frame.Cmd != "pong" {
		t.Fatalf("reconnect pushed offline state: %#v %v", frame, err)
	}
	unread, err = clients[1].GetUnreadSummary(context.Background())
	if err != nil || unread.Count != 2 {
		t.Fatal(unread, err)
	}
	history, err := clients[1].ListPrivateMessagesPage(context.Background(), a, 20, 0)
	if err != nil || len(history) != 3 || history[0].Content != "while offline" {
		t.Fatal(history, err)
	}

}

func receiveProtocolMessage(t *testing.T, ws *sdk.WebSocketClient) (sdk.WSMessage, error) {
	t.Helper()
	select {
	case message, ok := <-ws.Receive():
		if !ok {
			return sdk.WSMessage{}, fmt.Errorf("message stream closed")
		}
		return message, nil
	case <-time.After(5 * time.Second):
		t.Fatal("message stream timed out")
		return sdk.WSMessage{}, nil
	}
}
