package unit_test

import (
	"testing"

	"gochat/pkg/config"
	"gochat/pkg/mysql"
	"gochat/logic/model"
)

// 使用测试数据中的用户ID
// admin=1, alice=2, bob=3, charlie=4, diana=5, eve=6, frank=7, grace=8
// 已有好友关系: alice<->bob, alice<->charlie, alice<->diana, bob<->charlie, eve<->frank
// 测试使用 ID=8(grace) 作为新增测试用户

func TestFriendModel(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 使用测试数据中的用户 (frank=7, grace=8)
	userID := int64(7)  // frank
	friendID := int64(8) // grace

	// 清理可能存在的测试好友关系
	defer func() {
		mysql.Client.Where("user_id = ? AND friend_id = ?", userID, friendID).Delete(&model.Friend{})
		mysql.Client.Where("user_id = ? AND friend_id = ?", friendID, userID).Delete(&model.Friend{})
	}()

	// 测试前验证：frank 应该没有好友 grace
	isFriend, _ := model.IsFriend(userID, friendID)
	if isFriend {
		t.Skip("frank and grace are already friends, skipping test")
		return
	}

	// 测试添加好友
	friend := &model.Friend{
		UserID:   userID,
		FriendID: friendID,
		Remark:   "测试好友",
	}

	if err := model.CreateFriend(friend); err != nil {
		t.Fatalf("create friend failed: %v", err)
	}

	if friend.ID == 0 {
		t.Error("friend ID should not be 0 after create")
	}

	t.Logf("Created friend relationship with ID: %d", friend.ID)

	// 测试获取好友列表
	friends, err := model.GetFriends(userID)
	if err != nil {
		t.Fatalf("get friends failed: %v", err)
	}

	// frank 原本有一个好友 (eve)，现在应该有 2 个
	if len(friends) < 1 {
		t.Errorf("expected at least 1 friend, got %d", len(friends))
	}

	// 测试获取特定好友关系
	found, err := model.GetFriend(userID, friendID)
	if err != nil {
		t.Fatalf("get friend failed: %v", err)
	}

	if found.Remark != "测试好友" {
		t.Errorf("expected remark 测试好友, got %s", found.Remark)
	}

	// 测试是否是好友
	isFriend, err = model.IsFriend(userID, friendID)
	if err != nil {
		t.Fatalf("check is friend failed: %v", err)
	}

	if !isFriend {
		t.Error("expected users to be friends")
	}

	// 测试更新备注
	if err := model.UpdateFriendRemark(userID, friendID, "更新备注"); err != nil {
		t.Fatalf("update friend remark failed: %v", err)
	}

	// 验证备注更新
	updated, err := model.GetFriend(userID, friendID)
	if err != nil {
		t.Fatalf("get updated friend failed: %v", err)
	}

	if updated.Remark != "更新备注" {
		t.Errorf("expected remark 更新备注, got %s", updated.Remark)
	}

	// 测试删除好友
	if err := model.DeleteFriend(userID, friendID); err != nil {
		t.Fatalf("delete friend failed: %v", err)
	}

	// 验证删除
	isFriend, err = model.IsFriend(userID, friendID)
	if err != nil {
		t.Fatalf("check is friend after delete failed: %v", err)
	}

	if isFriend {
		t.Error("expected users not to be friends after delete")
	}

	t.Log("Friend model test passed")
}

func TestFriendModelQueryExisting(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 测试查询已有的好友关系 (alice=2, bob=3)
	userID := int64(2)  // alice
	friendID := int64(3) // bob

	// 测试获取好友列表
	friends, err := model.GetFriends(userID)
	if err != nil {
		t.Fatalf("get friends failed: %v", err)
	}

	// alice 有 3 个好友: bob, charlie, diana
	if len(friends) < 3 {
		t.Errorf("expected at least 3 friends for alice, got %d", len(friends))
	}

	// 测试获取特定好友关系
	found, err := model.GetFriend(userID, friendID)
	if err != nil {
		t.Fatalf("get friend failed: %v", err)
	}

	// 验证备注内容 (alice 对 bob 的备注是 "我的同事")
	if found.Remark != "我的同事" {
		t.Errorf("expected remark 我的同事, got %s", found.Remark)
	}

	// 测试是否是好友
	isFriend, err := model.IsFriend(userID, friendID)
	if err != nil {
		t.Fatalf("check is friend failed: %v", err)
	}

	if !isFriend {
		t.Error("expected alice and bob to be friends")
	}

	t.Log("Query existing friends test passed")
}

func TestFriendModel双向关系(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 验证已有的双向好友关系 (alice=2, bob=3)
	userID := int64(2)  // alice
	friendID := int64(3) // bob

	// 获取 alice 的好友
	friends1, err := model.GetFriends(userID)
	if err != nil {
		t.Fatalf("get alice's friends failed: %v", err)
	}

	// 获取 bob 的好友
	friends2, err := model.GetFriends(friendID)
	if err != nil {
		t.Fatalf("get bob's friends failed: %v", err)
	}

	// alice 和 bob 应该互为好友
	// alice 应该能看到 bob
	foundAlice := false
	for _, f := range friends1 {
		if f.FriendID == friendID {
			foundAlice = true
			break
		}
	}
	if !foundAlice {
		t.Error("alice should have bob as friend")
	}

	// bob 应该能看到 alice
	foundBob := false
	for _, f := range friends2 {
		if f.FriendID == userID {
			foundBob = true
			break
		}
	}
	if !foundBob {
		t.Error("bob should have alice as friend")
	}

	t.Logf("Bidirectional friend test passed: alice has %d friends, bob has %d friends", len(friends1), len(friends2))
}
