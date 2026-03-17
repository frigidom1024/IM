package unit_test

import (
	"testing"

	"gochat/pkg/config"
	"gochat/pkg/mysql"
	"gochat/logic/model"
)

// 使用测试数据中的群组
// 群组ID: 1=开发团队(owner:2), 2=产品讨论组(owner:3), 3=周末约饭群(owner:5), 4=技术交流(owner:6)
// 测试使用已有群组进行查询和更新测试

func TestGroupModel(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 使用测试数据中的群组 (开发团队=1, owner=alice=2)
	groupID := int64(1)

	// 测试获取群组
	found, err := model.GetGroupByID(groupID)
	if err != nil {
		t.Fatalf("get group failed: %v", err)
	}

	if found.Name != "开发团队" {
		t.Errorf("expected name 开发团队, got %s", found.Name)
	}

	if found.OwnerID != 2 {
		t.Errorf("expected owner_id 2, got %d", found.OwnerID)
	}

	// 测试获取用户拥有的群组 (alice=2)
	groups, err := model.GetGroupsByOwner(2)
	if err != nil {
		t.Fatalf("get groups by owner failed: %v", err)
	}

	if len(groups) != 1 {
		t.Errorf("expected 1 group for alice, got %d", len(groups))
	}

	// 测试更新群组
	originalName := found.Name
	if err := model.UpdateGroup(groupID, "更新后的群名", found.Avatar); err != nil {
		t.Fatalf("update group failed: %v", err)
	}

	// 验证更新
	updated, err := model.GetGroupByID(groupID)
	if err != nil {
		t.Fatalf("get updated group failed: %v", err)
	}

	if updated.Name != "更新后的群名" {
		t.Errorf("expected name 更新后的群名, got %s", updated.Name)
	}

	// 恢复原名称
	model.UpdateGroup(groupID, originalName, found.Avatar)

	t.Log("Group model test passed")
}

func TestGroupModelCreateAndDelete(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 使用测试数据中的用户 (frank=7)
	ownerID := int64(7)

	// 测试创建群组
	group := &model.Group{
		Name:       "临时测试群",
		Avatar:     "http://example.com/test.png",
		OwnerID:    ownerID,
		MaxMembers: 100,
	}

	if err := model.CreateGroup(group); err != nil {
		t.Fatalf("create group failed: %v", err)
	}

	if group.ID == 0 {
		t.Error("group ID should not be 0 after create")
	}

	// 清理测试数据
	defer func() {
		mysql.Client.Where("name = ?", "临时测试群").Delete(&model.Group{})
	}()

	t.Logf("Created group with ID: %d", group.ID)

	// 验证创建
	found, err := model.GetGroupByID(group.ID)
	if err != nil {
		t.Fatalf("get created group failed: %v", err)
	}

	if found.Name != "临时测试群" {
		t.Errorf("expected name 临时测试群, got %s", found.Name)
	}

	// 测试删除群组
	if err := model.DeleteGroup(group.ID); err != nil {
		t.Fatalf("delete group failed: %v", err)
	}

	// 验证删除
	_, err = model.GetGroupByID(group.ID)
	if err == nil {
		t.Error("expected error when getting deleted group")
	}

	t.Log("Group create/delete test passed")
}

func TestGroupTransfer(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建测试群组
	group := &model.Group{
		Name:    "待转让群组",
		OwnerID: 7, // frank
	}

	if err := model.CreateGroup(group); err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	defer func() {
		mysql.Client.Where("name = ?", "待转让群组").Delete(&model.Group{})
	}()

	// 原群主
	oldOwnerID := int64(7)
	// 新群主
	newOwnerID := int64(8) // grace

	// 测试转让群组
	if err := model.TransferGroup(group.ID, newOwnerID); err != nil {
		t.Fatalf("transfer group failed: %v", err)
	}

	// 验证转让
	updated, err := model.GetGroupByID(group.ID)
	if err != nil {
		t.Fatalf("get updated group failed: %v", err)
	}

	if updated.OwnerID != newOwnerID {
		t.Errorf("expected new owner_id %d, got %d", newOwnerID, updated.OwnerID)
	}

	// 恢复原群主
	model.TransferGroup(group.ID, oldOwnerID)

	t.Log("Group transfer test passed")
}
