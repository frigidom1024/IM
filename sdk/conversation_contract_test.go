package sdk_test

import (
	"context"
	"gochat/sdk"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConversationHTTPHistoryAndWholeRead(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			if len(body) != 0 {
				t.Errorf("read body must be empty: %s", body)
			}
			io.WriteString(w, `{"code":0}`)
			return
		}
		if r.URL.Path == "/api/conversation/unread" {
			io.WriteString(w, `{"code":0,"data":{"count":2,"unread_list":[{"id":"private:2","unread_count":2,"latest_message_id":"9007199254740993"}]}}`)
			return
		}
		io.WriteString(w, `{"code":0,"data":[{"id_string":"9007199254740993","id":9007199254740993,"content":"hello"}]}`)
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	ctx := context.Background()
	private, err := client.ListPrivateMessagesPage(ctx, 2, 10, 5)
	if err != nil || len(private) != 1 || private[0].ID != 9007199254740993 {
		t.Fatal(private, err)
	}
	if _, err = client.ListGroupMessagesPage(ctx, 7, 20, 0); err != nil {
		t.Fatal(err)
	}
	if err = client.MarkConversationRead(ctx, "private:2"); err != nil {
		t.Fatal(err)
	}
	if err = client.MarkReadConversation(ctx, 0, 7); err != nil {
		t.Fatal(err)
	}
	unread, err := client.GetUnreadSummary(ctx)
	if err != nil || len(unread.UnreadList) != 1 || unread.UnreadList[0].ID != "private:2" || unread.UnreadList[0].LatestMessageID != 9007199254740993 {
		t.Fatal(unread, err)
	}
	want := []string{"GET /api/conversation/private:2/messages?limit=10&offset=5", "GET /api/conversation/group:7/messages?limit=20&offset=0", "POST /api/conversation/private:2/read", "POST /api/conversation/group:7/read", "GET /api/conversation/unread"}
	if len(paths) != len(want) {
		t.Fatal(paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("path=%s want=%s", paths[i], want[i])
		}
	}
}
