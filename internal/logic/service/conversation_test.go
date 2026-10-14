package service

import (
	"gochat/internal/logic/model"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConversationPreview(t *testing.T) {
	if messagePreview(&model.Message{MsgType: model.MsgTypeImage, Content: "secret"}) != "[image]" {
		t.Fatal("media preview")
	}
	if messagePreview(&model.Message{MsgType: model.MsgTypeText, Content: "secret", IsRevoked: true}) != "" {
		t.Fatal("revoked content leaked")
	}
	preview := messagePreview(&model.Message{MsgType: model.MsgTypeText, Content: strings.Repeat("消息", 400)})
	if len(preview) > 512 || !utf8.ValidString(preview) {
		t.Fatal("invalid bounded preview")
	}
}
func TestConversationOwnerKeys(t *testing.T) {
	for _, scope := range []string{"private:0", "private:01", "private:1", "group:-1", "other:2"} {
		if _, err := ConversationKey(1, scope); err == nil {
			t.Fatal(scope)
		}
	}
	key, err := ConversationKey(1, "private:2")
	if err != nil || key.FromID != 1 || key.ToID != 2 || key.GroupID != 0 {
		t.Fatal(key, err)
	}
	key, err = ConversationKey(1, "group:2")
	if err != nil || key.FromID != 1 || key.ToID != 0 || key.GroupID != 2 {
		t.Fatal(key, err)
	}
}

func TestMessageValidationNeedsNoDatabase(t *testing.T) {
	service := NewMessageService(nil)
	for _, message := range []*model.Message{nil, {ID: 1, FromID: 1, ToID: 1, MsgType: "text", Content: "self"}, {ID: 1, FromID: 1, ToID: 2, GroupID: 3, MsgType: "text", Content: "both"}, {ID: 1, FromID: 1, ToID: 2, MsgType: "unsupported", Content: "bad"}, {ID: 1, FromID: 1, ToID: 2, MsgType: "text", Content: "  "}} {
		if err := service.Save(nil, message); err == nil {
			t.Fatal("invalid message accepted")
		}
	}
}
