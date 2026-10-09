package unit_test

import (
	"testing"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/mysql"
)

func TestUserModel(t *testing.T) {
	// 初始化
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建仓储实例
	userRepo := registry.NewUserRepository()

	// 清理测试数据
	mysql.Client.Where("username = ?", "test_user_model").Delete(&model.User{})

	// 测试创建用户
	user := &model.User{
		Username: "test_user_model",
		Password: "hashed_password",
		Nickname: "测试用户",
		Status:   0,
	}

	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if user.ID == 0 {
		t.Error("user ID should not be 0 after create")
	}

	t.Logf("Created user with ID: %d", user.ID)

	// 测试查询用户
	found, err := userRepo.GetByUsername("test_user_model")
	if err != nil {
		t.Fatalf("get user failed: %v", err)
	}

	if found.Nickname != "测试用户" {
		t.Errorf("expected nickname 测试用户, got %s", found.Nickname)
	}

	// 测试更新状态
	if err := userRepo.UpdateStatus(user.ID, 1); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// 验证状态更新
	updated, err := userRepo.GetByID(user.ID)
	if err != nil {
		t.Fatalf("get updated user failed: %v", err)
	}

	if updated.Status != 1 {
		t.Errorf("expected status 1, got %d", updated.Status)
	}

	// 测试更新资料
	if err := userRepo.UpdateProfile(user.ID, "新昵称", ""); err != nil {
		t.Fatalf("update profile failed: %v", err)
	}

	// 验证资料更新
	updated, err = userRepo.GetByID(user.ID)
	if err != nil {
		t.Fatalf("get updated user failed: %v", err)
	}

	if updated.Nickname != "新昵称" {
		t.Errorf("expected nickname 新昵称, got %s", updated.Nickname)
	}

	// 清理测试数据
	mysql.Client.Delete(user)

	t.Log("User model test passed")
}
