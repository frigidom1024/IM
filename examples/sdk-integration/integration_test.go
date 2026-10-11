package integration_test

import (
	"context"
	"gochat/sdk"
	"os"
	"testing"
	"time"
)

// Requires a running GoChat stack. Supply a builtin user's credentials.
func TestBuiltinSDKIntegration(t *testing.T) {
	base := os.Getenv("GOCHAT_URL")
	if base == "" {
		t.Skip("set GOCHAT_URL, GOCHAT_TEST_USERNAME and GOCHAT_TEST_PASSWORD")
	}
	username, password := os.Getenv("GOCHAT_TEST_USERNAME"), os.Getenv("GOCHAT_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Fatal("test credentials required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := sdk.NewClient(base, sdk.WithTimeout(5*time.Second), sdk.WithWebSocketURL(os.Getenv("GOCHAT_WS_URL")))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := client.Login(ctx, sdk.LoginRequest{Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	user := client.WithToken(auth.Token)
	if _, err = user.GetUserInfo(ctx, auth.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = user.ListFriends(ctx); err != nil {
		t.Fatal(err)
	}
	ws, err := user.ConnectWebSocket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	// Receive has no context parameter; the deadline closes the socket instead.
	stop := context.AfterFunc(ctx, func() { ws.Close() })
	defer stop()
	if err = ws.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err = ws.Receive(); err != nil {
		t.Fatal(err)
	}
}
