package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	drivermysql "github.com/go-sql-driver/mysql"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/migration"
	"gochat/internal/pkg/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

// Each test creates its own schema using the embedded migrations. The supplied
// DSN only selects an explicitly configured test instance, never a target DB.
func conversationFixture(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GOCHAT_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOCHAT_MIGRATION_TEST_DSN to an isolated MySQL 8.0 instance")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	cfg.MultiStatements = true
	cfg.ParseTime = true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("gochat_conversation_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg.DBName = name
	sqlDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err = migration.New(sqlDB).Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	previous := mysql.Client
	mysql.Client = db
	t.Cleanup(func() { mysql.Client = previous })
	for _, u := range []model.User{{ID: 1, Username: "alice"}, {ID: 2, Username: "bob"}, {ID: 3, Username: "carol"}, {ID: 4, Username: "dave"}} {
		if err = db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func requireDB(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func conversationRow(t *testing.T, db *gorm.DB, owner, peer, group int64) model.Conversation {
	t.Helper()
	var row model.Conversation
	requireDB(t, db.Where("from_id=? AND to_id=? AND group_id=?", owner, peer, group).First(&row).Error)
	return row
}

type failProjection struct {
	registry.ConversationRepository
	calls int
}

func (r *failProjection) Record(tx *gorm.DB, key model.Conversation, msg *model.Message, incr int64) error {
	r.calls++
	if r.calls == 2 {
		return errors.New("injected recipient projection failure")
	}
	return r.ConversationRepository.Record(tx, key, msg, incr)
}
func TestMessageAndConversationRollbackAndIdempotency(t *testing.T) {
	db := conversationFixture(t)
	requireDB(t, db.Create(&model.Friend{UserID: 1, FriendID: 2}).Error)
	repo := registry.NewConversationRepository()
	messages := registry.NewMessageRepository()
	failing := NewMessagingService(messages, &failProjection{ConversationRepository: repo})
	seq := "first"
	msg := &model.Message{ID: 101, FromID: 1, ToID: 2, MsgType: "text", Content: "hello", ClientSeq: &seq}
	if err := persistForTest(failing, msg); err == nil {
		t.Fatal("failure was not propagated")
	}
	for _, table := range []string{"message", "conversation", "private_pair"} {
		var n int64
		requireDB(t, db.Table(table).Count(&n).Error)
		if n != 0 {
			t.Fatalf("%s write escaped rollback", table)
		}
	}
	sender := NewMessagingService(messages, repo)
	requireDB(t, persistForTest(sender, msg))
	if row := conversationRow(t, db, 1, 2, 0); row.UnreadCount != 0 || *row.LastMessageID != 101 {
		t.Fatal(row)
	}
	if row := conversationRow(t, db, 2, 1, 0); row.UnreadCount != 1 {
		t.Fatal(row)
	}
	retry := &model.Message{ID: 102, FromID: 1, ToID: 2, MsgType: "text", Content: "hello", ClientSeq: &seq}
	if err := persistForTest(sender, retry); err == nil {
		t.Fatal("duplicate was inserted")
	}
	existing, err := sender.GetByClientSeq(1, seq)
	requireDB(t, err)
	if existing == nil || existing.ID != 101 || conversationRow(t, db, 2, 1, 0).UnreadCount != 1 {
		t.Fatal("retry changed projection")
	}
	// Legacy aggregates are inactive even though they remain in the schema.
	var n int64
	requireDB(t, db.Table("unread_count").Count(&n).Error)
	if n != 0 {
		t.Fatal("legacy aggregate was written")
	}
}
func TestConversationScopeHistoryAndReadIsolation(t *testing.T) {
	db := conversationFixture(t)
	repo := registry.NewConversationRepository()
	messages := registry.NewMessageRepository()
	svc := NewConversationService(repo, messages)
	history, err := svc.History(1, "private:2", 10, 0)
	requireDB(t, err)
	if len(history) != 0 {
		t.Fatal(history)
	}
	list, err := svc.List(1, 10, 0)
	requireDB(t, err)
	if len(list) != 0 {
		t.Fatal("history opened an empty row")
	}
	requireDB(t, svc.Open(1, "private:2"))
	requireDB(t, svc.Open(1, "private:2"))
	pinned := true
	draft := "owner draft"
	requireDB(t, svc.Patch(1, "private:2", &pinned, nil, &draft))
	requireDB(t, db.Create(&model.Friend{UserID: 1, FriendID: 2}).Error)
	requireDB(t, persistForTest(NewMessagingService(messages, repo), &model.Message{ID: 201, FromID: 1, ToID: 2, MsgType: "text", Content: "private"}))
	if row := conversationRow(t, db, 2, 1, 0); row.Pinned || row.Draft != "" {
		t.Fatal("preferences crossed owners")
	}
	total, items, err := svc.UnreadSummary(2)
	requireDB(t, err)
	if total != 1 || len(items) != 1 || items[0].ID != "private:1" {
		t.Fatal(total, items)
	}
	_, err = svc.MarkRead(1, "private:2")
	requireDB(t, err)
	if conversationRow(t, db, 2, 1, 0).UnreadCount != 1 {
		t.Fatal("read crossed owners")
	}
	read, err := svc.MarkRead(2, "private:1")
	requireDB(t, err)
	if read.ReadAt == nil || read.UnreadCount != 0 {
		t.Fatal(read)
	}
	if _, err = svc.History(3, "group:10", 10, 0); err == nil {
		t.Fatal("nonmember history allowed")
	}
}
func TestGroupProjectionMembershipAndPreferencePreservation(t *testing.T) {
	db := conversationFixture(t)
	repo := registry.NewConversationRepository()
	messages := registry.NewMessageRepository()
	svc := NewConversationService(repo, messages)
	sender := NewMessagingService(messages, repo)
	requireDB(t, db.Create(&model.Group{ID: 10, Name: "team", OwnerID: 1, MaxMembers: 10}).Error)
	requireDB(t, db.Create(&model.GroupMember{GroupID: 10, UserID: 1, Role: model.RoleOwner}).Error)
	requireDB(t, (GroupAdminService{}).Invite(10, 1, 2))
	rows, err := svc.List(2, 10, 0)
	requireDB(t, err)
	if len(rows) != 0 {
		t.Fatal("joining an empty group opened a row")
	}
	requireDB(t, persistForTest(sender, &model.Message{ID: 301, FromID: 1, GroupID: 10, MsgType: "text", Content: "group"}))
	if conversationRow(t, db, 1, 0, 10).UnreadCount != 0 || conversationRow(t, db, 2, 0, 10).UnreadCount != 1 {
		t.Fatal("group fanout counts")
	}
	requireDB(t, (GroupAdminService{}).Invite(10, 1, 3))
	joined := conversationRow(t, db, 3, 0, 10)
	if joined.UnreadCount != 0 || *joined.LastMessageID != 301 {
		t.Fatal("join replayed historical unread", joined)
	}
	muted := true
	draft := "preserve"
	requireDB(t, svc.Patch(2, "group:10", nil, &muted, &draft))
	requireDB(t, (GroupAdminService{}).Remove(10, 1, 2))
	rows, err = svc.List(2, 10, 0)
	requireDB(t, err)
	if len(rows) != 0 {
		t.Fatal("removed group still visible")
	}
	total, _, err := svc.UnreadSummary(2)
	requireDB(t, err)
	if total != 0 {
		t.Fatal("removed group counted unread")
	}
	if _, err = svc.MarkRead(2, "group:10"); err == nil {
		t.Fatal("removed member read permitted")
	}
	requireDB(t, persistForTest(sender, &model.Message{ID: 302, FromID: 1, GroupID: 10, MsgType: "text", Content: "after removal"}))
	removed := conversationRow(t, db, 2, 0, 10)
	if removed.UnreadCount != 0 || removed.Draft != "preserve" || !removed.Muted {
		t.Fatal(removed)
	}
	requireDB(t, (GroupAdminService{}).Invite(10, 1, 2))
	joined = conversationRow(t, db, 2, 0, 10)
	if joined.UnreadCount != 0 || *joined.LastMessageID != 302 || joined.Draft != "preserve" {
		t.Fatal(joined)
	}
	requireDB(t, (GroupAdminService{}).Dissolve(10, 1))
	for _, owner := range []int64{1, 2, 3} {
		rows, err = svc.List(owner, 10, 0)
		requireDB(t, err)
		if len(rows) != 0 {
			t.Fatal("dissolved scope visible")
		}
	}
}
func TestReadLockPreservesMessagesCommittedAfterRead(t *testing.T) {
	db := conversationFixture(t)
	repo := registry.NewConversationRepository()
	messages := registry.NewMessageRepository()
	requireDB(t, db.Create(&[]model.Friend{{UserID: 1, FriendID: 2}, {UserID: 2, FriendID: 1}}).Error)
	sender := NewMessagingService(messages, repo)
	requireDB(t, persistForTest(sender, &model.Message{ID: 401, FromID: 1, ToID: 2, MsgType: "text", Content: "before read"}))
	tx := db.Begin()
	requireDB(t, tx.Error)
	defer tx.Rollback()
	_, err := repo.MarkRead(tx, model.Conversation{FromID: 2, ToID: 1})
	requireDB(t, err)
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		done <- persistForTest(sender, &model.Message{ID: 402, FromID: 1, ToID: 2, MsgType: "text", Content: "during read"})
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("send bypassed locked row: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	requireDB(t, tx.Commit().Error)
	select {
	case err := <-done:
		requireDB(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("send did not resume")
	}
	row := conversationRow(t, db, 2, 1, 0)
	if row.UnreadCount != 1 || *row.LastMessageID != 402 {
		t.Fatal("new message was marked read", row)
	}
}

func persistForTest(s *MessagingService, m *model.Message) error { _, err := s.Send(m); return err }
