package sdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gochat/sdk"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestConversationPreferencesAndStringMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		if r.Method == "GET" {
			if r.URL.RawQuery != "limit=20&offset=5" {
				t.Error(r.URL.RawQuery)
			}
			io.WriteString(w, `{"code":0,"data":[{"id":"private:2","last_message":{"id":"9007199254740993","content":"hello"},"pinned":true}]}`)
			return
		}
		if r.Method != "PATCH" || r.URL.Path != "/api/conversation/private:2" {
			t.Error(r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["pinned"] != false || body["draft"] != "" || len(body) != 2 {
			t.Error(body)
		}
		io.WriteString(w, `{"code":0}`)
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	list, err := client.ListConversations(context.Background(), 20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].LastMessage.ID != 9007199254740993 {
		t.Fatal(list)
	}
	pinned, draft := false, ""
	if err = client.UpdateConversation(context.Background(), "private:2", sdk.ConversationPatch{Pinned: &pinned, Draft: &draft}); err != nil {
		t.Fatal(err)
	}
}

func TestGroupManagementContracts(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/api/group/search" {
			if r.URL.Query().Get("q") != "R&D / team" {
				t.Error(r.URL.RawQuery)
			}
			io.WriteString(w, `{"code":0,"data":[{"id":7,"announcement":"welcome"}]}`)
			return
		}
		if r.URL.Path == "/api/group/7/messages/90/receipts" || r.Method == "GET" {
			io.WriteString(w, `{"code":0,"data":[{"user_id":2,"nickname":"A","last_read_message_id":"90","role":0}]}`)
			return
		}
		if r.Method == "PATCH" {
			var data map[string]any
			json.NewDecoder(r.Body).Decode(&data)
			if data["nickname"] != "" || data["unmute"] != true || data["role"] != float64(0) {
				t.Error(data)
			}
		}
		io.WriteString(w, `{"code":0}`)
	}))
	defer server.Close()
	ctx := context.Background()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	groups, err := client.SearchGroups(ctx, "R&D / team", 20, 0)
	if err != nil || len(groups) != 1 || groups[0].Announcement != "welcome" {
		t.Fatal(groups, err)
	}
	if err = client.InviteGroupMember(ctx, 7, 2); err != nil {
		t.Fatal(err)
	}
	if err = client.TransferGroupOwner(ctx, 7, 2); err != nil {
		t.Fatal(err)
	}
	if err = client.UpdateGroupAnnouncement(ctx, 7, "welcome"); err != nil {
		t.Fatal(err)
	}
	nickname, role := "", int8(0)
	if err = client.UpdateGroupMember(ctx, 7, 2, sdk.GroupMemberPatch{Nickname: &nickname, Role: &role, Unmute: true}); err != nil {
		t.Fatal(err)
	}
	members, err := client.ListGroupMembersPage(ctx, 7, 20, 0)
	if err != nil || members[0].LastReadMessageID != 90 {
		t.Fatal(members, err)
	}
	if _, err = client.ListGroupReadReceipts(ctx, 7, 90, 20, 0); err != nil {
		t.Fatal(err)
	}
	if err = client.DissolveGroup(ctx, 7); err != nil {
		t.Fatal(err)
	}
	expected := []string{"GET /api/group/search", "POST /api/group/7/invite", "POST /api/group/7/transfer", "PUT /api/group/7/announcement", "PATCH /api/group/7/members/2", "GET /api/group/7/members", "GET /api/group/7/messages/90/receipts", "DELETE /api/group/7"}
	if len(calls) != len(expected) {
		t.Fatal(calls)
	}
	for i := range expected {
		if calls[i] != expected[i] {
			t.Errorf("call %d=%s", i, calls[i])
		}
	}
}

func TestResumeUploadUsesServerOffsetAndAcknowledgedProgress(t *testing.T) {
	var appended string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/upload/session/session-1":
			io.WriteString(w, `{"code":0,"data":{"id":"session-1","size":6,"offset":3,"name":"data.txt","type":"file","mime":"text/plain"}}`)
		case r.Method == "PATCH":
			if r.Header.Get("Upload-Offset") != "3" || r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Error(r.Header)
			}
			data, _ := io.ReadAll(r.Body)
			appended = string(data)
			io.WriteString(w, `{"code":0,"data":{"id":"session-1","size":6,"offset":6}}`)
		case r.Method == "POST" && r.URL.Path == "/api/upload/session/session-1/complete":
			io.WriteString(w, `{"code":0,"data":{"url":"/api/file/file-1/download","size":6}}`)
		default:
			t.Error(r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	var progress int64
	meta, err := client.UploadFile(context.Background(), sdk.UploadFileRequest{SessionID: "session-1", File: strings.NewReader("abcdef"), Size: 6, Progress: func(sent, total int64) {
		if total != 6 {
			t.Error(total)
		}
		progress = sent
	}})
	if err != nil {
		t.Fatal(err)
	}
	if appended != "def" || progress != 6 || meta.Size != 6 {
		t.Fatal(appended, progress, meta)
	}
}

func TestAuthenticatedDownloadAndThumbnailStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		switch r.URL.Path {
		case "/api/file/file-1/download":
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "streamed file")
		case "/api/file/file-1/thumbnail":
			w.Header().Set("Content-Type", "image/jpeg")
			io.WriteString(w, "image bytes")
		default:
			w.WriteHeader(403)
			io.WriteString(w, `{"code":40300,"message":"forbidden"}`)
		}
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	response, err := client.DownloadFile(context.Background(), "file-1")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(data) != "streamed file" {
		t.Fatal(string(data))
	}
	response, err = client.DownloadThumbnail(context.Background(), "file-1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatal(response.Header)
	}
	_, err = client.DownloadFile(context.Background(), "denied")
	var apiErr *sdk.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 40300 {
		t.Fatal(err)
	}
}

func TestFailedChunkIsNotReplayedAndCanResume(t *testing.T) {
	offset, patches := int64(0), 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/upload/session" {
			io.WriteString(w, `{"code":0,"data":{"id":"retry-session","size":6,"offset":0}}`)
			return
		}
		if r.Method == "GET" {
			fmt.Fprintf(w, `{"code":0,"data":{"id":"retry-session","size":6,"offset":%d}}`, offset)
			return
		}
		if r.Method == "PATCH" {
			patches++
			got, _ := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
			if got != offset {
				t.Errorf("offset=%d wanted=%d", got, offset)
			}
			data, _ := io.ReadAll(r.Body)
			if patches == 1 {
				offset = 3
				w.WriteHeader(503)
				io.WriteString(w, `{"code":503,"message":"response lost"}`)
				return
			}
			if string(data) != "def" {
				t.Error(string(data))
			}
			offset += int64(len(data))
			io.WriteString(w, `{"code":0,"data":{"id":"retry-session","size":6,"offset":6}}`)
			return
		}
		io.WriteString(w, `{"code":0,"data":{"size":6}}`)
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"), sdk.WithReadRetry(5, 0))
	_, err := client.UploadFile(context.Background(), sdk.UploadFileRequest{File: strings.NewReader("abcdef"), Size: 6, Name: "a.txt", Type: "file", MIME: "text/plain"})
	var uploadErr *sdk.UploadSessionError
	if !errors.As(err, &uploadErr) || uploadErr.SessionID != "retry-session" || patches != 1 {
		t.Fatalf("patches=%d err=%v", patches, err)
	}
	meta, err := client.UploadFile(context.Background(), sdk.UploadFileRequest{File: strings.NewReader("abcdef"), Size: 6, SessionID: uploadErr.SessionID})
	if err != nil || meta.Size != 6 || patches != 2 {
		t.Fatal(meta, err, patches)
	}
}
func TestResumableChunksAreBoundedToEightMiB(t *testing.T) {
	size, offset := int64((8<<20)+1), int64(0)
	var chunks []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			n, _ := io.Copy(io.Discard, r.Body)
			chunks = append(chunks, n)
			offset += n
		}
		if r.URL.Path == "/api/upload/session/big/complete" {
			fmt.Fprintf(w, `{"code":0,"data":{"size":%d}}`, size)
			return
		}
		fmt.Fprintf(w, `{"code":0,"data":{"id":"big","size":%d,"offset":%d}}`, size, offset)
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	_, err := client.UploadFile(context.Background(), sdk.UploadFileRequest{File: bytes.NewReader(make([]byte, size)), Size: size, SessionID: "big"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0] != 8<<20 || chunks[1] != 1 {
		t.Fatal(chunks)
	}
}
