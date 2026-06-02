package test

import (
	"gochat/pkg/snowflake"
	"gochat/pkg/util"
	"testing"
)

func TestSnowflakeID(t *testing.T) {
	if err := snowflake.Init(1, 1); err != nil {
		t.Fatalf("init snowflake failed: %v", err)
	}

	// 生成100个ID，检查唯一性
	ids := make(map[int64]bool)
	for i := 0; i < 100; i++ {
		id, err := snowflake.NextID()
		if err != nil {
			t.Fatalf("generate id failed: %v", err)
		}
		if ids[id] {
			t.Errorf("duplicate id: %d", id)
		}
		ids[id] = true
	}

	t.Logf("generated %d unique IDs", len(ids))
}

func TestBcrypt(t *testing.T) {
	password := "test123456"

	hash, err := util.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}

	if !util.CheckPassword(hash, password) {
		t.Error("password check failed")
	}

	if util.CheckPassword(hash, "wrong") {
		t.Error("should not match wrong password")
	}

	t.Log("bcrypt test passed")
}

func TestJwt(t *testing.T) {
	token, err := util.GenerateToken(1, "test", 1)

	if err != nil {

		t.Fatalf("generate token failed: %v", err)

	}

	claims, err := util.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token failed: %v", err)
	}

	t.Logf("claims: %v", claims)
	if claims.UserID != 1 {
		t.Errorf("expected user ID 1, got %d", claims.UserID)
	}
	if claims.UserName != "test" {
		t.Errorf("expected username test, got %s", claims.UserName)
	}
	t.Logf("token: %s", token)
}
