package sdk

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientExternalAuthAndAuthenticatedRequest(t *testing.T) {
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth":
			if r.Method != http.MethodPost {
				t.Fatalf("auth method = %s, want POST", r.Method)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"token":"gochat-token","user":{"id":42,"name":"Alice"}}}`)
		case "/api/friend/list":
			if r.Header.Get("Authorization") != "Bearer gochat-token" {
				t.Fatalf("friend auth = %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"code":0,"data":[{"id":1,"user_id":42,"friend_id":43}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := client.ExternalAuth(context.Background(), "Bearer external-token")
	if err != nil {
		t.Fatal(err)
	}
	if auth.Token != "gochat-token" || auth.User.ID != 42 {
		t.Fatalf("auth response = %#v", auth)
	}

	friends, err := client.WithToken(auth.Token).ListFriends(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || friends[0].FriendID != 43 {
		t.Fatalf("friends = %#v", friends)
	}
	if !strings.EqualFold(authHeaders[0], "Bearer external-token") {
		t.Fatalf("external auth header = %q", authHeaders[0])
	}
}

func TestClientRequiresToken(t *testing.T) {
	client, err := NewClient("https://gochat.example.test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListFriends(context.Background())
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("error = %v, want ErrMissingToken", err)
	}
}

func TestNormalizeToken(t *testing.T) {
	cases := map[string]string{
		"token":            "token",
		"Bearer token":     "token",
		"bearer token":     "token",
		"  Bearer token  ": "token",
		"":                 "",
	}
	for input, want := range cases {
		if got := NormalizeToken(input); got != want {
			t.Errorf("NormalizeToken(%q) = %q, want %q", input, got, want)
		}
	}
}
