//go:build integration

package test

import (
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/etcd"
	"testing"
	"time"
)

func TestEtcdConnection(t *testing.T) {
	cfg := config.MustLoad("../config/config-dev.yaml")

	etcd.Init(&cfg.Etcd)

	defer etcd.Close()

	reg, err := etcd.NewServiceRegistry("test", "server1", "127.0.0.1:8080")

	if err != nil {
		t.Fatalf("Failed to create service registry: %v", err)
	}
	defer reg.Close()

	time.Sleep(time.Second)

	discovery := etcd.NewServiceDiscovery("test")
	services, err := discovery.GetServices()

	if err != nil {

		t.Fatalf("Failed to get services: %v", err)

	}

	if len(services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(services))
	}
	if services[0] != "127.0.0.1:8080" {
		t.Fatalf("Expected 127.0.0.1:8080, got %s", services[0])
	}
	t.Logf("Successfully discovered service: %s", services[0])
}
