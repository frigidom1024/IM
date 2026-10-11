package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/minio"
	"gochat/internal/pkg/mysql"
	"gochat/internal/pkg/util"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Uses a dedicated disposable database supplied by the E2E runner, never the
// development database. All feature assertions use authenticated HTTP routes.
func TestP1ConversationsAndResumableMediaHTTP(t *testing.T) {
	dsn := os.Getenv("GOCHAT_P1_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOCHAT_P1_TEST_DSN to a disposable MySQL database")
	}
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := mysql.Client
	mysql.Client = db
	defer func() { mysql.Client = previous; sqlDB, _ := db.DB(); sqlDB.Close() }()
	if err = db.AutoMigrate(&model.User{}, &model.Friend{}, &model.Group{}, &model.GroupMember{}, &model.Message{}, &model.UnreadCount{}, &model.ConversationPreference{}, &model.UploadSession{}, &model.MediaAsset{}); err != nil {
		t.Fatal(err)
	}
	if err = logger.Init("error", "stdout"); err != nil {
		t.Fatal(err)
	}
	util.Init("p1-integration-secret")
	for _, id := range []int64{9101, 9102, 9103} {
		user := model.User{ID: id, Username: fmt.Sprintf("p1-%d", id), Nickname: fmt.Sprintf("User %d", id)}
		if err = db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, table := range []string{"media_asset", "upload_session", "conversation_preference", "unread_count", "message", "group_member", "group", "friend", "user"} {
			db.Exec("DELETE FROM `" + table + "`")
		}
	}()
	// Fixture has an outgoing-only conversation and one joined group, independent
	// of unread aggregates. This catches implementations deriving only unread scopes.
	if err = db.Create(&model.Message{ID: 900001, FromID: 9101, ToID: 9102, MsgType: "text", Content: "outgoing", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	group := model.Group{ID: 9201, OwnerID: 9101, Name: "P1 group", MaxMembers: 10}
	db.Create(&group)
	db.Create(&model.GroupMember{GroupID: 9201, UserID: 9101, Role: model.RoleOwner})
	endpoint := os.Getenv("GOCHAT_TEST_S3")
	if endpoint == "" {
		endpoint = "127.0.0.1:19000"
	}
	store, err := minio.NewClient(&config.MinIOConfig{Endpoint: endpoint, AccessKeyID: "test", SecretAccessKey: "test-secret", Bucket: "p1-tests"})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(registry.NewUserRepository(), registry.NewFriendRepository(), registry.NewGroupRepository(), registry.NewGroupMemberRepository(), registry.NewMessageRepository(), registry.NewMessageReadRepository(), store, "builtin", "", 0, 0, nil, nil)
	server := httptest.NewServer(router.Handler())
	defer server.Close()
	token1, _ := util.GenerateToken(9101, "alice", 1)
	token2, _ := util.GenerateToken(9102, "bob", 1)
	request := func(method, path, token, contentType string, body io.Reader, headers map[string]string, expected int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != expected {
			t.Fatalf("%s %s: status %d: %s", method, path, res.StatusCode, data)
		}
		return data
	}
	data := request("GET", "/api/conversation", token1, "", nil, nil, 200)
	var list struct{ Data []model.Conversation }
	if err = json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 2 {
		t.Fatalf("conversations=%s", data)
	}
	request("PATCH", "/api/conversation/private:9102", token1, "application/json", strings.NewReader(`{"pinned":true,"muted":true,"draft":"private draft"}`), nil, 200)
	data = request("GET", "/api/conversation?limit=1", token1, "", nil, nil, 200)
	json.Unmarshal(data, &list)
	if len(list.Data) != 1 || list.Data[0].ID != "private:9102" || !list.Data[0].Pinned || list.Data[0].Draft != "private draft" || list.Data[0].LastMessage.Content != "outgoing" {
		t.Fatalf("preferences=%s", data)
	}
	data = request("GET", "/api/conversation", token2, "", nil, nil, 200)
	json.Unmarshal(data, &list)
	if len(list.Data) != 1 || list.Data[0].Pinned || list.Data[0].Draft != "" {
		t.Fatalf("cross-user preferences leaked: %s", data)
	}
	request("PATCH", "/api/conversation/group:9201", token2, "application/json", strings.NewReader(`{"pinned":true}`), nil, 403)
	data = request("POST", "/api/upload/session", token1, "application/json", strings.NewReader(`{"name":"hello.txt","type":"file","mime":"text/plain","size":11}`), nil, 200)
	var session struct{ Data model.UploadSession }
	json.Unmarshal(data, &session)
	path := "/api/upload/session/" + session.Data.ID
	request("GET", path, token2, "", nil, nil, 404)
	request("POST", path+"/complete", token1, "application/json", strings.NewReader(`{}`), nil, 409)
	request("PATCH", path, token1, "application/octet-stream", strings.NewReader("hello "), map[string]string{"Upload-Offset": "0"}, 200)
	data = request("GET", path, token1, "", nil, nil, 200)
	json.Unmarshal(data, &session)
	if session.Data.Offset != 6 {
		t.Fatalf("resume offset=%s", data)
	}
	request("PATCH", path, token1, "application/octet-stream", strings.NewReader("hello "), map[string]string{"Upload-Offset": "0"}, 409)
	request("PATCH", path, token1, "application/octet-stream", strings.NewReader("world"), map[string]string{"Upload-Offset": "6"}, 200)
	data = request("POST", path+"/complete", token1, "application/json", strings.NewReader(`{}`), nil, 200)
	var meta struct{ Data minio.FileMeta }
	json.Unmarshal(data, &meta)
	if meta.Data.Size != 11 || meta.Data.Name != "hello.txt" {
		t.Fatalf("file metadata=%s", data)
	}
	if downloaded := request("GET", meta.Data.URL, token1, "", nil, nil, 200); !bytes.Equal(downloaded, []byte("hello world")) {
		t.Fatalf("downloaded=%q", downloaded)
	}
	if ranged := request("GET", meta.Data.URL, token1, "", nil, map[string]string{"Range": "bytes=6-10"}, 206); string(ranged) != "world" {
		t.Fatalf("range=%q", ranged)
	}
	request("GET", meta.Data.URL, token2, "", nil, nil, 403)
	again := request("POST", path+"/complete", token1, "application/json", strings.NewReader(`{}`), nil, 200)
	if !bytes.Equal(data, again) {
		t.Fatalf("completion isn't idempotent: %s / %s", data, again)
	}
	request("DELETE", path, token1, "", nil, nil, 500)
}
