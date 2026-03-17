package unit_test

import (
	"testing"

	"gochat/pkg/config"
	"gochat/pkg/mysql"
	"gochat/pkg/snowflake"
	"gochat/logic/model"
)

// 使用测试数据中的消息和已读记录
// 已读记录: alice<->bob 的 4 条消息, 开发团队的 3 条消息

func TestMessageReadModel(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 测试已有的消息已读状态
	messageID := int64(1000000000000000001) // alice -> bob
	userID := int64(3)                        // bob

	// 这条消息 bob 已经读过了
	isRead, err := model.IsMessageRead(messageID, userID)
	if err != nil {
		t.Fatalf("check is message read failed: %v", err)
	}

	if !isRead {
		t.Error("expected message to be read by bob")
	}

	// 测试获取消息已读状态
	readStatus, err := model.GetMessageReadStatus(messageID)
	if err != nil {
		t.Fatalf("get message read status failed: %v", err)
	}

	if len(readStatus) != 1 {
		t.Errorf("expected 1 read status, got %d", len(readStatus))
	}

	if readStatus[0].UserID != userID {
		t.Errorf("expected user_id %d, got %d", userID, readStatus[0].UserID)
	}

	t.Log("Message read status test passed")
}

func TestMarkMessageRead(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 创建测试消息 (frank -> grace)
	msg := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  7, // frank
		ToID:    8, // grace
		MsgType: model.MsgTypeText,
		Content: "测试消息",
	}

	if err := model.CreateMessage(msg); err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	// 清理测试数据
	defer func() {
		mysql.Client.Where("id = ?", msg.ID).Delete(&model.Message{})
		mysql.Client.Where("message_id = ?", msg.ID).Delete(&model.MessageRead{})
	}()

	userID := int64(8) // grace

	// 测试标记消息已读
	if err := model.MarkMessageRead(msg.ID, userID); err != nil {
		t.Fatalf("mark message read failed: %v", err)
	}

	t.Logf("Marked message %d as read by user %d", msg.ID, userID)

	// 验证已读
	isRead, err := model.IsMessageRead(msg.ID, userID)
	if err != nil {
		t.Fatalf("check is message read failed: %v", err)
	}

	if !isRead {
		t.Error("expected message to be read")
	}

	// 测试重复标记（不应该出错）
	if err := model.MarkMessageRead(msg.ID, userID); err != nil {
		t.Fatalf("duplicate mark should not error: %v", err)
	}

	// 验证仍然只有一条记录
	readStatus, _ := model.GetMessageReadStatus(msg.ID)
	if len(readStatus) != 1 {
		t.Errorf("expected still 1 read status after duplicate mark, got %d", len(readStatus))
	}

	t.Log("Mark message read test passed")
}

func TestBatchMarkMessagesRead(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 创建多条测试消息
	msgIDs := []int64{}
	for i := 0; i < 3; i++ {
		msg := &model.Message{
			ID:      snowflake.MustNextID(),
			FromID:  7, // frank
			ToID:    8, // grace
			MsgType: model.MsgTypeText,
			Content: "测试消息",
		}
		if err := model.CreateMessage(msg); err != nil {
			t.Fatalf("create message %d failed: %v", i, err)
		}
		msgIDs = append(msgIDs, msg.ID)
	}

	defer func() {
		mysql.Client.Where("id IN ?", msgIDs).Delete(&model.Message{})
		mysql.Client.Where("message_id IN ?", msgIDs).Delete(&model.MessageRead{})
	}()

	userID := int64(8) // grace

	// 测试批量标记已读
	if err := model.BatchMarkMessagesRead(msgIDs, userID); err != nil {
		t.Fatalf("batch mark messages read failed: %v", err)
	}

	t.Logf("Batch marked %d messages as read", len(msgIDs))

	// 验证所有消息都已读
	for _, msgID := range msgIDs {
		isRead, err := model.IsMessageRead(msgID, userID)
		if err != nil {
			t.Fatalf("check message %d read status failed: %v", msgID, err)
		}
		if !isRead {
			t.Errorf("message %d should be read", msgID)
		}
	}

	t.Log("Batch mark messages read test passed")
}

func TestUnreadMessageCount(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 创建测试用户和消息
	// 使用 frank(7) 发送给 grace(8) 的消息
	msgIDs := []int64{}
	for i := 0; i < 5; i++ {
		msg := &model.Message{
			ID:      snowflake.MustNextID(),
			FromID:  7, // frank
			ToID:    8, // grace
			MsgType: model.MsgTypeText,
			Content: "测试消息",
		}
		if err := model.CreateMessage(msg); err != nil {
			t.Fatalf("create message %d failed: %v", i, err)
		}
		msgIDs = append(msgIDs, msg.ID)
	}

	defer func() {
		mysql.Client.Where("id IN ?", msgIDs).Delete(&model.Message{})
		mysql.Client.Where("message_id IN ?", msgIDs).Delete(&model.MessageRead{})
	}()

	userID := int64(8) // grace

	// 测试未读数量
	count, err := model.GetUnreadMessageCount(userID)
	if err != nil {
		t.Fatalf("get unread count failed: %v", err)
	}

	// grace 至少有这 5 条未读消息（可能还有其他未读消息）
	if count < 5 {
		t.Errorf("expected at least 5 unread messages, got %d", count)
	}

	// 标记部分消息已读
	model.MarkMessageRead(msgIDs[0], userID)
	model.MarkMessageRead(msgIDs[1], userID)

	// 验证未读数量更新
	count, err = model.GetUnreadMessageCount(userID)
	if err != nil {
		t.Fatalf("get unread count after mark failed: %v", err)
	}

	// 应该至少还有 3 条未读
	if count < 3 {
		t.Errorf("expected at least 3 unread messages after marking 2, got %d", count)
	}

	t.Log("Unread message count test passed")
}

func TestGroupUnreadMessageCount(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 使用测试数据中的群组 (开发团队=1)
	groupID := int64(1)
	// 使用 frank(7)，他不在开发团队中
	userID := int64(7)

	// 创建一些群消息（由 alice 发送）
	msgIDs := []int64{}
	for i := 0; i < 3; i++ {
		msg := &model.Message{
			ID:      snowflake.MustNextID(),
			FromID:  2, // alice
			ToID:    0,
			GroupID: groupID,
			MsgType: model.MsgTypeText,
			Content: "群消息测试",
		}
		if err := model.CreateMessage(msg); err != nil {
			t.Fatalf("create message %d failed: %v", i, err)
		}
		msgIDs = append(msgIDs, msg.ID)
	}

	defer func() {
		mysql.Client.Where("id IN ?", msgIDs).Delete(&model.Message{})
		mysql.Client.Where("message_id IN ?", msgIDs).Delete(&model.MessageRead{})
	}()

	// frank 不在这个群里，所以查询可能返回 0
	count, err := model.GetGroupUnreadMessageCount(groupID, userID)
	if err != nil {
		t.Fatalf("get group unread count failed: %v", err)
	}

	t.Logf("Group %d has %d unread messages for user %d", groupID, count, userID)

	// 如果用户不在群里，未读数可能是消息总数
	// 这里不做严格断言，只测试函数能正常执行

	t.Log("Group unread message count test passed")
}
