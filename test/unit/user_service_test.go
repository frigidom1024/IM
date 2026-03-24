package unit_test

import (
	"testing"

	"gochat/logic/model"
	"gochat/logic/registry"
	"gochat/logic/service"
	"gochat/pkg/config"
	"gochat/pkg/mysql"
)

func TestUserService(t *testing.T) {
	// 初始化数据库
	cfg := config.MustLoad("../../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {
		t.Fatalf("init mysql failed: %v", err)
	}
	defer mysql.Close()

	// 创建服务实例
	userRepo := registry.NewUserRepository()
	userService := service.NewUserService(userRepo)

	// 清理测试数据
	mysql.Client.Where("username = ?", "test_service_user").Delete(&model.User{})

	// 测试注册
	registerReq := &service.RegisterReq{
		Username: "test_service_user",
		Password: "123456",
		Nickname: "服务测试用户",
	}

	userRes, err := userService.Register(registerReq)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if userRes.Username != "test_service_user" {
		t.Errorf("expected username test_service_user, got %s", userRes.Username)
	}

	if userRes.ID == 0 {
		t.Error("user ID should not be 0 after register")
	}

	t.Logf("Registered user: ID=%d, Username=%s", userRes.ID, userRes.Username)

	// 测试重复注册
	_, err = userService.Register(registerReq)
	if err == nil {
		t.Error("register should fail for duplicate username")
	}
	t.Logf("Expected duplicate registration error: %v", err)

	// 测试登录
	loginReq := &service.LoginReq{
		Username: "test_service_user",
		Password: "123456",
	}

	loginRes, token, err := userService.Login(loginReq)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty after login")
	}

	if loginRes.Status != 1 {
		t.Errorf("expected status 1 (online), got %d", loginRes.Status)
	}

	t.Logf("Login successful: token=%s", token[:20]+"...")

	// 测试获取用户信息
	userInfo, err := userService.GetUserInfo(userRes.ID)
	if err != nil {
		t.Fatalf("get user info failed: %v", err)
	}

	if userInfo.Nickname != "服务测试用户" {
		t.Errorf("expected nickname 服务测试用户, got %s", userInfo.Nickname)
	}

	// 测试更新资料
	err = userService.UpdateProfile(userRes.ID, "新昵称", "")
	if err != nil {
		t.Fatalf("update profile failed: %v", err)
	}

	// 验证更新
	updated, err := userService.GetUserInfo(userRes.ID)
	if err != nil {
		t.Fatalf("get updated user failed: %v", err)
	}

	if updated.Nickname != "新昵称" {
		t.Errorf("expected nickname 新昵称, got %s", updated.Nickname)
	}

	// 测试错误密码登录
	wrongReq := &service.LoginReq{
		Username: "test_service_user",
		Password: "wrongpassword",
	}

	_, _, err = userService.Login(wrongReq)
	if err == nil {
		t.Error("login should fail with wrong password")
	}
	t.Logf("Expected wrong password error: %v", err)

	// 测试不存在的用户登录
	notExistReq := &service.LoginReq{
		Username: "notexist_user",
		Password: "123456",
	}

	_, _, err = userService.Login(notExistReq)
	if err == nil {
		t.Error("login should fail for non-existent user")
	}
	t.Logf("Expected user not exists error: %v", err)

	// 测试更新状态
	err = userService.UpdateStatus(userRes.ID, 2)
	if err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// 验证状态更新
	updated, err = userService.GetUserInfo(userRes.ID)
	if err != nil {
		t.Fatalf("get updated user failed: %v", err)
	}

	if updated.Status != 2 {
		t.Errorf("expected status 2 (busy), got %d", updated.Status)
	}

	// 清理测试数据
	mysql.Client.Delete(&model.User{}, userRes.ID)

	t.Log("User service test passed")
}
