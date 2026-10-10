package handler

import (
	"context"
	"testing"

	"gochat/internal/connect"
	"gochat/internal/logic/model"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/snowflake"
)

type chatMessageServiceStub struct {
	duplicate       *model.Message
	stored          *model.Message
	unreadTargets   []model.UnreadTarget
	sendCallCount   int
	unreadCallCount int
}

func (s *chatMessageServiceStub) Send(*model.Message) error { s.sendCallCount++; return nil }
func (s *chatMessageServiceStub) SendWithUnread(message *model.Message, targets []model.UnreadTarget) error {
	s.stored = message
	s.unreadTargets = targets
	s.unreadCallCount++
	return nil
}
func (s *chatMessageServiceStub) GetByClientSeq(int64, string) (*model.Message, error) {
	return s.duplicate, nil
}
func (s *chatMessageServiceStub) GetByID(int64) (*model.Message, error)                { return nil, nil }
func (s *chatMessageServiceStub) IncrUnread(int64, int64, int64, *model.Message) error { return nil }

func (s *chatMessageServiceStub) GetMembers(int64) ([]*model.GroupMember, error) { return nil, nil }

type chatConnServerStub struct {
	messages []connect.Message
}

func (s *chatConnServerStub) Send(_ string, message connect.Message) error {
	s.messages = append(s.messages, message)
	return nil
}
func (s *chatConnServerStub) IsOnline(string) bool { return false }

type chatGroupServiceStub struct {
	members []*model.GroupMember
}

func (s *chatGroupServiceStub) GetMembers(int64) ([]*model.GroupMember, error) {
	return s.members, nil
}

func TestChatHandlerReturnsDuplicateAckWithoutRewriting(t *testing.T) {
	messageService := &chatMessageServiceStub{duplicate: &model.Message{ID: 123}}
	connection := &chatConnServerStub{}
	handler := NewChatHandler(messageService, &chatGroupServiceStub{}, connection)

	message := connect.NewRawMessage(connect.CmdMsg, "client-seq-1", &connect.ChatMsgData{
		FromID:  7,
		ToID:    8,
		Type:    model.MsgTypeText,
		Content: "retry",
	})
	if err := handler.Handle(context.Background(), "7", message); err != nil {
		t.Fatal(err)
	}
	if messageService.unreadCallCount != 0 || messageService.sendCallCount != 0 {
		t.Fatalf("duplicate retry rewrote message: sends=%d unread=%d", messageService.sendCallCount, messageService.unreadCallCount)
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

func TestChatHandlerStoresClientSeqAndUnreadTarget(t *testing.T) {
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
		FromID:  7,
		ToID:    8,
		Type:    model.MsgTypeText,
		Content: "new",
	})
	if err := handler.Handle(context.Background(), "7", message); err != nil {
		t.Fatal(err)
	}
	if messageService.stored == nil || messageService.stored.ClientSeq == nil || *messageService.stored.ClientSeq != "client-seq-2" {
		t.Fatalf("stored client seq = %#v", messageService.stored)
	}
	if len(messageService.unreadTargets) != 1 || messageService.unreadTargets[0].UserID != 8 || messageService.unreadTargets[0].FromID != 7 || messageService.unreadTargets[0].GroupID != 0 {
		t.Fatalf("unread targets = %#v", messageService.unreadTargets)
	}
}
