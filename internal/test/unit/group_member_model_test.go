package unit_test

import (
	"testing"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/mysql"
)

// 使用测试数据中的群组和成员
// 群组1(开发团队): 成员 alice=2(owner), bob=3(admin), charlie=4, eve=6
// 群组2(产品讨论组): 成员 bob=3(owner), alice=2(admin), diana=5
// 群组3(周末约饭群): 成员 diana=5(owner), alice=2, bob=3, charlie=4
// 群组4(技术交流): 成员 eve=6(owner), frank=7(admin), grace=8

func TestGroupMemberModel(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建仓储实例
	groupMemberRepo := registry.NewGroupMemberRepository()

	// 使用测试数据中的群组 (开发团队=1)
	groupID := int64(1)

	// 测试获取群成员列表
	members, err := groupMemberRepo.GetByGroupID(groupID)
	if err != nil {
		t.Fatalf("get group members failed: %v", err)
	}

	// 开发团队有 4 个成员
	if len(members) != 4 {
		t.Errorf("expected 4 members in 开发团队, got %d", len(members))
	}

	// 测试获取特定群成员 (alice=2)
	userID := int64(2)
	found, err := groupMemberRepo.Get(groupID, userID)
	if err != nil {
		t.Fatalf("get group member failed: %v", err)
	}

	// alice 是群主
	if found.Role != model.RoleOwner {
		t.Errorf("expected role %d(owner), got %d", model.RoleOwner, found.Role)
	}

	// 测试是否是群成员
	isMember, err := groupMemberRepo.IsMember(groupID, userID)
	if err != nil {
		t.Fatalf("check is member failed: %v", err)
	}

	if !isMember {
		t.Error("expected alice to be group member")
	}

	// 测试获取群成员数量
	count, err := groupMemberRepo.GetCount(groupID)
	if err != nil {
		t.Fatalf("get member count failed: %v", err)
	}

	if count != 4 {
		t.Errorf("expected member count 4, got %d", count)
	}

	// 测试更新成员角色 (bob=3 是管理员，改为普通成员)
	bobID := int64(3)
	if err := groupMemberRepo.UpdateRole(groupID, bobID, model.RoleMember); err != nil {
		t.Fatalf("update member role failed: %v", err)
	}

	// 验证角色更新
	updated, err := groupMemberRepo.Get(groupID, bobID)
	if err != nil {
		t.Fatalf("get updated member failed: %v", err)
	}

	if updated.Role != model.RoleMember {
		t.Errorf("expected role %d, got %d", model.RoleMember, updated.Role)
	}

	// 恢复角色
	groupMemberRepo.UpdateRole(groupID, bobID, model.RoleAdmin)

	t.Log("GroupMember model test passed")
}

func TestGroupMemberAddAndRemove(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建仓储实例
	groupRepo := registry.NewGroupRepository()
	groupMemberRepo := registry.NewGroupMemberRepository()

	// 创建测试群组
	group := &model.Group{
		Name:    "临时测试群",
		OwnerID: 7, // frank
	}

	if err := groupRepo.Create(group); err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	defer func() {
		mysql.Client.Where("name = ?", "临时测试群").Delete(&model.Group{})
		mysql.Client.Where("group_id = ?", group.ID).Delete(&model.GroupMember{})
	}()

	// 添加群主 (frank=7)
	ownerMember := &model.GroupMember{
		GroupID: group.ID,
		UserID:  7,
		Role:    model.RoleOwner,
	}

	if err := groupMemberRepo.Add(ownerMember); err != nil {
		t.Fatalf("add owner member failed: %v", err)
	}

	// 添加普通成员 (grace=8)
	userMember := &model.GroupMember{
		GroupID: group.ID,
		UserID:  8,
		Role:    model.RoleMember,
	}

	if err := groupMemberRepo.Add(userMember); err != nil {
		t.Fatalf("add member failed: %v", err)
	}

	t.Logf("Added group members, group ID: %d", group.ID)

	// 验证成员数量
	count, _ := groupMemberRepo.GetCount(group.ID)
	if count != 2 {
		t.Errorf("expected 2 members, got %d", count)
	}

	// 测试移除群成员
	if err := groupMemberRepo.Remove(group.ID, 8); err != nil {
		t.Fatalf("remove member failed: %v", err)
	}

	// 验证移除
	isMember, err := groupMemberRepo.IsMember(group.ID, 8)
	if err != nil {
		t.Fatalf("check is member after remove failed: %v", err)
	}

	if isMember {
		t.Error("expected user not to be group member after remove")
	}

	// 验证成员数量更新
	count, err = groupMemberRepo.GetCount(group.ID)
	if err != nil {
		t.Fatalf("get member count after remove failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected member count 1 after remove, got %d", count)
	}

	t.Log("GroupMember add/remove test passed")
}

func TestGroupMemberGetUserGroups(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建仓储实例
	groupMemberRepo := registry.NewGroupMemberRepository()

	// 使用测试数据中的用户 (alice=2)
	// alice 参加了 3 个群组: 开发团队(owner), 产品讨论组(admin), 周末约饭群
	userID := int64(2)

	// 测试获取用户加入的群组
	userGroups, err := groupMemberRepo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("get user groups failed: %v", err)
	}

	// alice 应该在 3 个群组中
	if len(userGroups) != 3 {
		t.Errorf("expected 3 groups for alice, got %d", len(userGroups))
	}

	t.Logf("alice is in %d groups", len(userGroups))

	// 验证群组ID
	groupIDs := make(map[int64]bool)
	for _, ug := range userGroups {
		groupIDs[ug.GroupID] = true
	}

	expectedGroups := []int64{1, 2, 3} // 开发团队, 产品讨论组, 周末约饭群
	for _, eg := range expectedGroups {
		if !groupIDs[eg] {
			t.Errorf("alice should be in group %d", eg)
		}
	}

	t.Log("GetUserGroups test passed")
}
