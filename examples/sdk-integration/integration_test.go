package integration_test

import (
	"bytes"
	"context"
	"gochat/sdk"
	"io"
	"os"
	"strings"
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
	external := os.Getenv("GOCHAT_EXTERNAL_TOKEN")
	if external == "" && (username == "" || password == "") {
		t.Fatal("test credentials required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := sdk.NewClient(base, sdk.WithTimeout(5*time.Second), sdk.WithWebSocketURL(os.Getenv("GOCHAT_WS_URL")))
	if err != nil {
		t.Fatal(err)
	}
	var auth *sdk.AuthResponse
	if external != "" {
		auth, err = client.ExternalAuth(ctx, external)
	} else {
		auth, err = client.Login(ctx, sdk.LoginRequest{Username: username, Password: password})
	}
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
	if _, err = user.ListConversations(ctx, 50, 0); err != nil {
		t.Fatal(err)
	}
	progress := int64(0)
	meta, err := user.UploadFile(ctx, sdk.UploadFileRequest{File: bytes.NewReader([]byte("sdk consumer")), Name: "consumer.txt", Type: "file", MIME: "text/plain", Size: 12, Progress: func(done, total int64) { progress = done }})
	if err != nil {
		t.Fatal(err)
	}
	if progress != 12 {
		t.Fatalf("progress=%d", progress)
	}
	parts := strings.Split(meta.URL, "/")
	if len(parts) != 5 {
		t.Fatalf("download URL=%s", meta.URL)
	}
	response, err := user.DownloadFile(ctx, parts[3])
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || string(data) != "sdk consumer" {
		t.Fatalf("download=%q err=%v", data, readErr)
	}
	ws, err := user.ConnectWebSocket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	// Cancellation stops consumption and closes the client/streams.
	stop := context.AfterFunc(ctx, func() { ws.Close() })
	defer stop()
	if err = ws.Ping(); err != nil {
		t.Fatal(err)
	}
	select {
	case message, ok := <-ws.Receive():
		if !ok || message.Cmd != "pong" {
			t.Fatalf("unexpected receive: %#v, open=%t", message, ok)
		}
	case err, ok := <-ws.Errors():
		t.Fatalf("receive error: %v, open=%t", err, ok)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
