package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"gochat/internal/connect"
	"gochat/internal/logic/model"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/snowflake"
)

type chatMessageServiceStub struct {
	duplicate     *model.Message
	stored        *model.Message
	sendCallCount int
	recipients    []int64
}

func (s *chatMessageServiceStub) Send(message *model.Message) ([]int64, error) {
	s.stored = message
	s.sendCallCount++
	if s.recipients != nil {
		return s.recipients, nil
	}
	return []int64{message.ToID}, nil
}
func (s *chatMessageServiceStub) GetByClientSeq(int64, string) (*model.Message, error) {
	return s.duplicate, nil
}
func (s *chatMessageServiceStub) GetByID(int64) (*model.Message, error) { return nil, nil }

func (s *chatMessageServiceStub) GetMembers(int64) ([]*model.GroupMember, error) { return nil, nil }

type chatConnServerStub struct {
	messages []connect.Message
	targets  []string
	online   bool
}

func (s *chatConnServerStub) Send(user string, message connect.Message) error {
	s.messages = append(s.messages, message)
	s.targets = append(s.targets, user)
	return nil
}
func (s *chatConnServerStub) IsOnline(string) bool { return s.online }

type chatGroupServiceStub struct {
	members []*model.GroupMember
}

func (s *chatGroupServiceStub) GetMembers(int64) ([]*model.GroupMember, error) {
	return s.members, nil
}

func TestChatHandlerReturnsDuplicateAckWithoutRewriting(t *testing.T) {
	messageService := &chatMessageServiceStub{duplicate: &model.Message{ID: 123, FromID: 7, ToID: 8, MsgType: "text", Content: "retry"}}
	connection := &chatConnServerStub{}
	handler := NewChatHandler(messageService, &chatGroupServiceStub{}, connection)

	message := connect.NewRawMessage(connect.CmdMsg, "client-seq-1", &connect.ChatMsgData{
		ConversationID: "private:8",
		Type:           model.MsgTypeText,
		Content:        "retry",
	})
	if err := handler.Handle(context.Background(), "7", message); err != nil {
		t.Fatal(err)
	}
	if messageService.sendCallCount != 0 {
		t.Fatalf("duplicate retry rewrote message: sends=%d", messageService.sendCallCount)
	}
	if len(connection.messages) != 1 {
		t.Fatalf("sent messages = %d, want one duplicate ACK", len(connection.messages))
	}
	ack := connection.messages[0].(*connect.RawMessage)
	data := ack.Data.(map[string]any)
	if data["duplicate"] != true || data["message_id"] != int64(123) {
		t.Fatalf("duplicate ACK data = %#v", data)
	}
}

func TestChatHandlerStoresClientSeqAndRecipient(t *testing.T) {
	if err := logger.Init("error", "stdout"); err != nil {
		t.Fatal(err)
	}
	defer logger.Sync()
	if err := snowflake.Init(1, 1); err != nil {
		t.Fatal(err)
	}
	messageService := &chatMessageServiceStub{}
	handler := NewChatHandler(messageService, &chatGroupServiceStub{}, &chatConnServerStub{})

	message := connect.NewRawMessage(connect.CmdMsg, "client-seq-2", &connect.ChatMsgData{
		ConversationID: "private:8",
		Type:           model.MsgTypeText,
		Content:        "new",
	})
	if err := handler.Handle(context.Background(), "7", message); err != nil {
		t.Fatal(err)
	}
	if messageService.stored == nil || messageService.stored.ClientSeq == nil || *messageService.stored.ClientSeq != "client-seq-2" {
		t.Fatalf("stored client seq = %#v", messageService.stored)
	}
	if messageService.stored.ToID != 8 || messageService.stored.FromID != 7 || messageService.stored.GroupID != 0 {
		t.Fatalf("message recipient = %#v", messageService.stored)
	}
}

func TestChatDeliveryUsesViewerScopeAndAuthenticatedAuthor(t *testing.T) {
	logger.Init("error", "stdout")
	snowflake.Init(1, 1)
	for _, test := range []struct {
		scope      string
		group      int64
		recipients []int64
		downScope  string
	}{
		{"private:8", 0, []int64{8}, "private:7"}, {"group:10", 10, []int64{8, 9}, "group:10"},
	} {
		t.Run(test.scope, func(t *testing.T) {
			messages := &chatMessageServiceStub{recipients: test.recipients}
			connection := &chatConnServerStub{online: true}
			handler := NewChatHandler(messages, &chatGroupServiceStub{members: []*model.GroupMember{{UserID: 7}}}, connection)
			// An obsolete, spoofed sender field cannot override the authenticated author.
			wire, _ := json.Marshal(map[string]any{"cmd": "msg", "seq": "scope-test", "data": map[string]any{"conversation_id": test.scope, "from_id": 999, "type": "text", "content": "hello"}})
			request, err := connect.ParseWsMsg(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err = handler.Handle(context.Background(), "7", request); err != nil {
				t.Fatal(err)
			}
			if messages.stored.FromID != 7 || messages.stored.GroupID != test.group {
				t.Fatal(messages.stored)
			}
			if len(connection.messages) != 1+len(test.recipients) {
				t.Fatal(connection.messages)
			}
			ack := connection.messages[0].(*connect.RawMessage).Data.(map[string]any)
			if ack["conversation_id"] != test.scope {
				t.Fatal(ack)
			}
			for i, frame := range connection.messages[1:] {
				data := frame.(*connect.RawMessage).Data.(map[string]any)
				if data["conversation_id"] != test.downScope || data["sender_id"] != "7" || data["id"] == "" {
					t.Fatal(data)
				}
				for _, field := range []string{"from_id", "to_id", "group_id"} {
					if _, ok := data[field]; ok {
						t.Fatalf("legacy field %s", field)
					}
				}
				if connection.targets[i+1] != strconv.FormatInt(test.recipients[i], 10) {
					t.Fatal(connection.targets)
				}
			}
		})
	}
}
func TestChatRejectsLegacyRoutingInvalidScopesAndSelfPrivate(t *testing.T) {
	logger.Init("error", "stdout")
	for _, payload := range []string{
		`{"from_id":7,"to_id":8,"type":"text","content":"old"}`,
		`{"conversation_id":"private:7","type":"text","content":"self"}`,
		`{"conversation_id":"group:0","type":"text","content":"bad"}`,
		`{"conversation_id":"private:08","type":"text","content":"bad"}`,
		`{"conversation_id":"group:9223372036854775808","type":"text","content":"bad"}`,
	} {
		messages := &chatMessageServiceStub{}
		connection := &chatConnServerStub{}
		handler := NewChatHandler(messages, nil, connection)
		frame, err := connect.ParseWsMsg([]byte(`{"cmd":"msg","seq":"invalid","data":` + payload + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if err = handler.Handle(context.Background(), "7", frame); err == nil {
			t.Fatal(payload)
		}
		if messages.sendCallCount != 0 || len(connection.messages) != 1 || connection.messages[0].Cmd() != "error" {
			t.Fatal(connection.messages)
		}
	}
}
func TestRetryCannotAcknowledgeDifferentConversation(t *testing.T) {
	logger.Init("error", "stdout")
	messages := &chatMessageServiceStub{duplicate: &model.Message{ID: 123, FromID: 7, ToID: 8, MsgType: "text", Content: "hello"}}
	connection := &chatConnServerStub{}
	handler := NewChatHandler(messages, nil, connection)
	for _, data := range []*connect.ChatMsgData{{ConversationID: "private:9", Type: "text", Content: "hello"}, {ConversationID: "private:8", Type: "text", Content: "changed"}} {
		if err := handler.Handle(context.Background(), "7", connect.NewRawMessage("msg", "same-seq", data)); err == nil {
			t.Fatal("conflicting retry accepted")
		}
	}
	if messages.sendCallCount != 0 {
		t.Fatal("conflict persisted")
	}
	for _, frame := range connection.messages {
		if frame.Cmd() != "error" {
			t.Fatal(frame)
		}
	}
}
