package registry

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gochat/internal/logic/model"
)

func TestLatestMessageSummaryUsesSessionPreview(t *testing.T) {
	createdAt := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	id, senderID, msgType, preview, gotAt := latestMessageSummary(&model.Message{
		ID:        123,
		FromID:    7,
		MsgType:   model.MsgTypeImage,
		Content:   `{"url":"/images/large-file"}`,
		CreatedAt: createdAt,
	})

	if id != 123 || senderID != 7 || msgType != model.MsgTypeImage {
		t.Fatalf("summary identity = %d/%d/%s", id, senderID, msgType)
	}
	if preview != "[image]" {
		t.Fatalf("preview = %q, want [image]", preview)
	}
	if !gotAt.Equal(createdAt) {
		t.Fatalf("created at = %v, want %v", gotAt, createdAt)
	}
}

func TestTruncatePreviewKeepsValidUTF8(t *testing.T) {
	preview := truncatePreview(strings.Repeat("消息", 400), 512)
	if len(preview) > 512 {
		t.Fatalf("preview bytes = %d, want <= 512", len(preview))
	}
	if !utf8.ValidString(preview) {
		t.Fatal("preview is not valid UTF-8")
	}
}
