package test

import (
	"testing"

	"gochat/pkg/config"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := config.Load("../config/config-dev.yaml")
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if cfg.Server.APIAddr != ":8080" {
		t.Errorf("expected APIAddr :8080, got %s", cfg.Server.APIAddr)
	}

	t.Logf("Config loaded successfully: %+v", cfg)
}
