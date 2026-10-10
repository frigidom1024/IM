package api

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRouterRunReadyBindsBeforeSignal(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, func() bool { return true })
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- router.RunReady(context.Background(), "127.0.0.1:0", ready) }()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("router did not report readiness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := router.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown error = %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("router did not stop")
	}
}
