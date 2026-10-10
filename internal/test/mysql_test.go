//go:build integration

package test

import (
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/mysql"
	"testing"
)

func TestMySQLConnection(t *testing.T) {
	cfg := config.MustLoad("../config/config-dev.yaml")
	if err := mysql.Init(&cfg.MySQL); err != nil {

		t.Fatalf("init mysql failed: %v", err)

	}
	defer mysql.Close()

	var result int
	if err := mysql.GetClient().Raw("SELECT 1").Scan(&result).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if result != 1 {
		t.Fatalf("query result error: expected 1, got %d", result)
	}
	t.Logf("query success: %d", result)
}
