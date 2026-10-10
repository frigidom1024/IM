package connect

import (
	"testing"
	"time"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"

	"gorm.io/gorm"
)

type offlineMessageRepoStub struct{}

func (offlineMessageRepoStub) Create(*model.Message) error                           { return nil }
func (offlineMessageRepoStub) CreateWithTx(*gorm.DB, *model.Message) error           { return nil }
func (offlineMessageRepoStub) FindByClientSeq(int64, string) (*model.Message, error) { return nil, nil }
func (offlineMessageRepoStub) GetByID(int64) (*model.Message, error)                 { return nil, nil }
func (offlineMessageRepoStub) GetPrivateMessages(int64, int64, int, int64) ([]*model.Message, error) {
	return nil, nil
}
func (offlineMessageRepoStub) GetGroupMessages(int64, int, int64) ([]*model.Message, error) {
	return nil, nil
}
func (offlineMessageRepoStub) Revoke(int64) error { return nil }
func (offlineMessageRepoStub) GetByUserID(int64, int, int64) ([]*model.Message, error) {
	return nil, nil
}
func (offlineMessageRepoStub) GetUnreadMessages(int64, int) ([]*model.Message, error) {
	return nil, nil
}

type offlineReadRepoStub struct{}

func (offlineReadRepoStub) IncrUnread(int64, int64, int64, *model.Message) error { return nil }
func (offlineReadRepoStub) IncrUnreadWithTx(*gorm.DB, int64, int64, int64, *model.Message) error {
	return nil
}
func (offlineReadRepoStub) GetUnreadCount(int64) (int64, error) { return 8, nil }
func (offlineReadRepoStub) GetUnreadList(int64) ([]*model.UnreadCount, error) {
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	return []*model.UnreadCount{
		{
			FromID:            7,
			UnreadCount:       5,
			LatestMessageID:   123,
			LatestSenderID:    7,
			LatestMessageType: model.MsgTypeText,
			LatestPreview:     "最后一条消息",
			LatestMessageAt:   &at,
		},
		{
			GroupID:           100,
			UnreadCount:       3,
			LatestMessageID:   456,
			LatestSenderID:    8,
			LatestMessageType: model.MsgTypeText,
			LatestPreview:     "群里的最新消息",
			LatestMessageAt:   &at,
		},
	}, nil
}
func (offlineReadRepoStub) ClearUnread(int64, int64, int64) error { return nil }

var _ registry.MessageRepository = offlineMessageRepoStub{}
var _ registry.MessageReadRepository = offlineReadRepoStub{}

func TestOfflineNotifyAggregatesByMessageScope(t *testing.T) {
	messageService := service.NewMessageService(offlineMessageRepoStub{}, offlineReadRepoStub{})
	notifier := NewOfflineNotifier(messageService).(*offlineNotifier)
	notify, err := notifier.GetOfflineNotify("42")
	if err != nil {
		t.Fatal(err)
	}
	if notify.TotalUnreadCount != 8 {
		t.Fatalf("total unread = %d, want 8", notify.TotalUnreadCount)
	}
	if len(notify.UnreadList) != 2 {
		t.Fatalf("scope count = %d, want 2", len(notify.UnreadList))
	}
	if notify.UnreadList[1].FromID != 0 || notify.UnreadList[1].GroupID != 100 {
		t.Fatalf("group scope = %#v", notify.UnreadList[1])
	}
	if notify.UnreadList[1].LatestMessage == nil || notify.UnreadList[1].LatestMessage.ID != 456 {
		t.Fatalf("latest group message = %#v", notify.UnreadList[1].LatestMessage)
	}
}
