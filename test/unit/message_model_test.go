package unit_test

import (
	"testing"

	"gochat/logic/model"
	"gochat/logic/registry"
	"gochat/pkg/config"
	"gochat/pkg/mysql"
	"gochat/pkg/snowflake"
)

// 使用测试数据中的消息
// 私聊消息ID: 1000000000000000001-1000000000000000009
// 群聊消息ID: 1000000000000000010-1000000000000000023
// alice=2, bob=3, charlie=4, diana=5, eve=6, frank=7, grace=8
// 群组: 1=开发团队, 2=产品讨论组, 3=周末约饭群, 4=技术交流

func TestMessageModel(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 创建仓储实例
	msgRepo := registry.NewMessageRepository()

	// 测试获取已存在的消息
	messageID := int64(1000000000000000001) // alice -> bob 的私聊
	found, err := msgRepo.GetByID(messageID)
	if err != nil {
		t.Fatalf("get message failed: %v", err)
	}

	if found.Content != "你好，鲍勃！最近怎么样？" {
		t.Errorf("expected content 你好，鲍勃！最近怎么样？, got %s", found.Content)
	}

	if found.MsgType != model.MsgTypeText {
		t.Errorf("expected msg_type %d, got %d", model.MsgTypeText, found.MsgType)
	}

	if found.FromID != 2 || found.ToID != 3 {
		t.Errorf("expected from_id=2, to_id=3, got from_id=%d, to_id=%d", found.FromID, found.ToID)
	}

	t.Log("Message model query test passed")
}

func TestMessageCreateAndRevoke(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 创建仓储实例
	msgRepo := registry.NewMessageRepository()

	// 创建测试消息
	msg := &model.Message{
		ID:      snowflake.MustNextID(),
		FromID:  7, // frank
		ToID:    8, // grace
		GroupID: 0,
		MsgType: model.MsgTypeText,
		Content: "这是一条测试消息",
	}

	if err := msgRepo.Create(msg); err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	// 清理测试数据
	defer func() {
		mysql.Client.Where("id = ?", msg.ID).Delete(&model.Message{})
	}()

	t.Logf("Created message with ID: %d", msg.ID)

	// 验证创建
	found, err := msgRepo.GetByID(msg.ID)
	if err != nil {
		t.Fatalf("get message failed: %v", err)
	}

	if found.Content != "这是一条测试消息" {
		t.Errorf("expected content 这是一条测试消息, got %s", found.Content)
	}

	// 测试撤回消息
	if err := msgRepo.Revoke(msg.ID); err != nil {
		t.Fatalf("revoke message failed: %v", err)
	}

	// 验证撤回
	revoked, err := msgRepo.GetByID(msg.ID)
	if err != nil {
		t.Fatalf("get revoked message failed: %v", err)
	}

	if !revoked.IsRevoked {
		t.Error("expected message to be revoked")
	}

	t.Log("Message create/revoke test passed")
}

func TestPrivateMessages(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建仓储实例
	msgRepo := registry.NewMessageRepository()

	// 测试获取 alice 和 bob 的私聊消息
	userID1 := int64(2) // alice
	userID2 := int64(3) // bob

	messages, err := msgRepo.GetPrivateMessages(userID1, userID2, 10, 0)
	if err != nil {
		t.Fatalf("get private messages failed: %v", err)
	}

	// alice 和 bob 有 4 条私聊消息
	if len(messages) != 4 {
		t.Errorf("expected 4 messages between alice and bob, got %d", len(messages))
	}

	// 验证消息内容（第一条应该是最新的）
	if len(messages) > 0 {
		t.Logf("Latest message: %s", messages[0].Content)
	}

	t.Log("Private messages test passed")
}

func TestGroupMessages(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建仓储实例
	msgRepo := registry.NewMessageRepository()

	// 测试获取开发团队的消息 (group_id=1)
	groupID := int64(1)

	messages, err := msgRepo.GetGroupMessages(groupID, 10, 0)
	if err != nil {
		t.Fatalf("get group messages failed: %v", err)
	}

	// 开发团队有 5 条群消息
	if len(messages) != 5 {
		t.Errorf("expected 5 messages in 开发团队, got %d", len(messages))
	}

	// 验证群消息属性
	if len(messages) > 0 {
		msg := messages[0]
		if msg.GroupID != groupID {
			t.Errorf("expected group_id %d, got %d", groupID, msg.GroupID)
		}
		if msg.ToID != 0 {
			t.Errorf("expected to_id 0 for group message, got %d", msg.ToID)
		}
		t.Logf("Latest group message: %s", msg.Content)
	}

	t.Log("Group messages test passed")
}

func TestMessageTypes(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	snowflake.Init(1, 1)
	defer mysql.Close()

	// 创建仓储实例
	msgRepo := registry.NewMessageRepository()

	// 测试不同类型的消息
	messageTypes := []struct {
		msgType  int8
		content  string
	}{
		{model.MsgTypeText, "文字消息测试"},
		{model.MsgTypeImage, "http://example.com/image.png"},
		{model.MsgTypeFile, "http://example.com/file.pdf"},
		{model.MsgTypeMention, "@frank 请查看"},
	}

	// 创建测试消息
	userID := int64(7) // frank
	defer func() {
		// 清理测试消息
		for _, mt := range messageTypes {
			mysql.Client.Where("from_id = ? AND content = ?", userID, mt.content).Delete(&model.Message{})
		}
	}()

	for _, mt := range messageTypes {
		msg := &model.Message{
			ID:      snowflake.MustNextID(),
			FromID:  userID,
			ToID:    8, // grace
			MsgType: mt.msgType,
			Content: mt.content,
		}

		if err := msgRepo.Create(msg); err != nil {
			t.Fatalf("create %d message failed: %v", mt.msgType, err)
		}

		found, err := msgRepo.GetByID(msg.ID)
		if err != nil {
			t.Fatalf("get %d message failed: %v", mt.msgType, err)
		}

		if found.Content != mt.content {
			t.Errorf("expected content %s, got %s", mt.content, found.Content)
		}

		if found.MsgType != mt.msgType {
			t.Errorf("expected msg_type %d, got %d", mt.msgType, found.MsgType)
		}
	}

	t.Log("Message types test passed")
}
