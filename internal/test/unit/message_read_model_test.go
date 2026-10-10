//go:build integration

package unit_test

import (
	"testing"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/mysql"
	"gochat/internal/pkg/snowflake"
)

func initMessageReadIntegration(t *testing.T) registry.MessageReadRepository {
	t.Helper()
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	t.Cleanup(func() { _ = mysql.Close() })
	return registry.NewMessageReadRepository()
}

func TestUnreadSummaryPrivateScope(t *testing.T) {
	repo := initMessageReadIntegration(t)
	userID, fromID := int64(8), int64(7)
	message := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  fromID,
		ToID:    userID,
		MsgType: model.MsgTypeText,
		Content: "private unread summary",
	}

	if err := repo.IncrUnread(userID, fromID, 0, message); err != nil {
		t.Fatalf("increment private unread failed: %v", err)
	}
	t.Cleanup(func() {
		mysql.Client.Where("user_id = ? AND from_id = ? AND group_id = ?", userID, fromID, 0).Delete(&model.UnreadCount{})
	})

	list, err := repo.GetUnreadList(userID)
	if err != nil {
		t.Fatalf("get unread summaries failed: %v", err)
	}
	var summary *model.UnreadCount
	for _, item := range list {
		if item.FromID == fromID && item.GroupID == 0 {
			summary = item
			break
		}
	}
	if summary == nil {
		t.Fatal("private unread summary not found")
	}
	if summary.UnreadCount < 1 || summary.LatestMessageID != message.ID {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestUnreadSummaryGroupsMessagesByGroup(t *testing.T) {
	repo := initMessageReadIntegration(t)
	userID, groupID := int64(8), int64(1)
	first := &model.Message{ID: snowflake.MustNextID(), FromID: 2, GroupID: groupID, MsgType: model.MsgTypeText, Content: "one"}
	second := &model.Message{ID: snowflake.MustNextID(), FromID: 3, GroupID: groupID, MsgType: model.MsgTypeText, Content: "two"}

	if err := repo.IncrUnread(userID, 2, groupID, first); err != nil {
		t.Fatalf("increment first group unread failed: %v", err)
	}
	if err := repo.IncrUnread(userID, 3, groupID, second); err != nil {
		t.Fatalf("increment second group unread failed: %v", err)
	}
	t.Cleanup(func() {
		mysql.Client.Where("user_id = ? AND from_id = ? AND group_id = ?", userID, 0, groupID).Delete(&model.UnreadCount{})
	})

	list, err := repo.GetUnreadList(userID)
	if err != nil {
		t.Fatalf("get group unread summaries failed: %v", err)
	}
	matches := 0
	for _, item := range list {
		if item.FromID == 0 && item.GroupID == groupID {
			matches++
			if item.UnreadCount < 2 || item.LatestMessageID != second.ID || item.LatestSenderID != second.FromID {
				t.Fatalf("group summary = %#v", item)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("group summary rows = %d, want 1", matches)
	}
}

func TestClearUnreadScope(t *testing.T) {
	repo := initMessageReadIntegration(t)
	userID, fromID := int64(8), int64(7)
	message := &model.Message{ID: snowflake.MustNextID(), FromID: fromID, ToID: userID, MsgType: model.MsgTypeText, Content: "clear me"}
	if err := repo.IncrUnread(userID, fromID, 0, message); err != nil {
		t.Fatalf("increment unread failed: %v", err)
	}
	t.Cleanup(func() {
		mysql.Client.Where("user_id = ? AND from_id = ? AND group_id = ?", userID, fromID, 0).Delete(&model.UnreadCount{})
	})

	if err := repo.ClearUnread(userID, fromID, 0); err != nil {
		t.Fatalf("clear unread failed: %v", err)
	}
	count, err := repo.GetUnreadCount(userID)
	if err != nil {
		t.Fatalf("get unread count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("unread count after clear = %d, want 0", count)
	}
}
