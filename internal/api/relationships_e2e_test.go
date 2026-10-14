package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"gochat/internal/connect"
	wsHandler "gochat/internal/connect/handler"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/snowflake"
	"gochat/sdk"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/mysql"
	"gochat/internal/pkg/util"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type relationshipFixture struct {
	t        *testing.T
	db       *gorm.DB
	server   *httptest.Server
	users    []model.User
	tokens   []string
	wsServer *httptest.Server
}

func (f *relationshipFixture) socket(user int) *websocket.Conn {
	f.t.Helper()
	snowflake.Init(0, 1)
	ws := connect.NewWsConnServer()
	ws.Registry().Register(connect.CmdMsg, wsHandler.NewChatHandler(service.NewMessagingService(registry.NewMessageRepository(), registry.NewConversationRepository()), service.NewGroupService(registry.NewGroupRepository(), registry.NewGroupMemberRepository()), ws))
	server := httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	f.t.Cleanup(func() { ws.Shutdown(context.Background()); server.Close() })
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"?token="+f.tokens[user], nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { conn.Close() })
	return conn
}

type relationReply struct {
	Cmd  string
	Seq  string
	Data struct {
		Code      int
		Message   string
		Duplicate bool
		MessageID string `json:"message_id_string"`
	}
}

func sendRelation(t *testing.T, conn *websocket.Conn, from, to int64, seq, kind, content string) relationReply {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	err := conn.WriteJSON(map[string]any{"cmd": "msg", "seq": seq, "data": map[string]any{"conversation_id": fmt.Sprintf("private:%d", to), "type": kind, "content": content}})
	if err != nil {
		t.Fatal(err)
	}
	var reply relationReply
	if err = conn.ReadJSON(&reply); err != nil {
		t.Fatal(err)
	}
	return reply
}
func TestSingleDirectionPrivateMessagingOverWebSocket(t *testing.T) {
	f := newRelationshipFixture(t)
	a, b := f.users[0].ID, f.users[1].ID
	conn := f.socket(0)
	if reply := sendRelation(t, conn, a, b, "no-friend", "text", "hello"); reply.Cmd != "error" || reply.Data.Code != 42010 {
		t.Fatalf("unadded private send allowed: %+v", reply)
	}
	f.call(0, "POST", "/api/friend/add", map[string]any{"friend_id": b}, 200)
	if reply := sendRelation(t, conn, a, b, "no-media", "image", `{"url":"test"}`); reply.Cmd != "error" || reply.Data.Code != 42011 {
		t.Fatalf("nontext intro allowed: %+v", reply)
	}
	first := sendRelation(t, conn, a, b, "first", "text", "hello")
	if first.Cmd != "msg_reply" {
		t.Fatalf("first intro failed: %+v", first)
	}
	retry := sendRelation(t, conn, a, b, "first", "text", "hello")
	if !retry.Data.Duplicate || retry.Data.MessageID != first.Data.MessageID {
		t.Fatalf("retry changed message: %+v", retry)
	}
	if reply := sendRelation(t, conn, a, b, "second", "text", "again"); reply.Cmd != "error" || reply.Data.Code != 42012 {
		t.Fatalf("second intro allowed: %+v", reply)
	}
	raw := f.call(1, "GET", "/api/conversation", nil, 200)
	var items []model.ConversationSummary
	json.Unmarshal(raw, &items)
	if len(items) != 1 || items[0].UnreadCount != 1 || items[0].SendPolicy != "unavailable" {
		t.Fatalf("recipient conversation missing: %s", raw)
	}
	f.call(0, "DELETE", fmt.Sprintf("/api/friend/%d", b), nil, 200)
	f.call(0, "POST", "/api/friend/add", map[string]any{"friend_id": b}, 200)
	if reply := sendRelation(t, conn, a, b, "after-readd", "text", "again"); reply.Cmd != "error" || reply.Data.Code != 42012 {
		t.Fatalf("readding reset quota: %+v", reply)
	}
	f.call(1, "POST", "/api/friend/add", map[string]any{"friend_id": a}, 200)
	for _, seq := range []string{"mutual-1", "mutual-2"} {
		if reply := sendRelation(t, conn, a, b, seq, "file", `{"name":"test"}`); reply.Cmd != "msg_reply" {
			t.Fatalf("mutual media restricted: %+v", reply)
		}
	}
	f.call(1, "DELETE", fmt.Sprintf("/api/friend/%d", a), nil, 200)
	if reply := sendRelation(t, conn, a, b, "after-unfriend", "text", "again"); reply.Cmd != "error" {
		t.Fatalf("unfriending lost restriction: %+v", reply)
	}
}
func TestConcurrentPrivateIntrosOnlyPersistOne(t *testing.T) {
	f := newRelationshipFixture(t)
	a, b := f.users[0].ID, f.users[1].ID
	f.call(0, "POST", "/api/friend/add", map[string]any{"friend_id": b}, 200)
	conns := []*websocket.Conn{f.socket(0), f.socket(0)}
	replies := make([]relationReply, 2)
	var wg sync.WaitGroup
	for i := range conns {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			replies[i] = sendRelation(t, conns[i], a, b, fmt.Sprintf("concurrent-%d", i), "text", "hello")
		}(i)
	}
	wg.Wait()
	success := 0
	for _, r := range replies {
		if r.Cmd == "msg_reply" {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent intros: %+v", replies)
	}
	raw := f.call(1, "GET", fmt.Sprintf("/api/conversation/private:%d/messages", a), nil, 200)
	var history []any
	json.Unmarshal(raw, &history)
	if len(history) != 1 {
		t.Fatalf("concurrent messages persisted: %s", raw)
	}
}

func newRelationshipFixture(t *testing.T) *relationshipFixture {
	t.Helper()
	dsn := os.Getenv("GOCHAT_RELATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOCHAT_RELATION_TEST_DSN to a disposable MySQL database")
	}
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := mysql.Client
	mysql.Client = db
	t.Cleanup(func() { mysql.Client = previous; pool, _ := db.DB(); pool.Close() })
	if err = db.AutoMigrate(&model.User{}, &model.Friend{}, &model.PrivatePair{}, &model.Group{}, &model.GroupMember{}, &model.GroupApplication{}, &model.Message{}, &model.Conversation{}); err != nil {
		t.Fatal(err)
	}
	logger.Init("error", "stdout")
	util.Init("relationship-e2e-secret")
	f := &relationshipFixture{t: t, db: db}
	t.Cleanup(func() {
		ids := make([]int64, 0, len(f.users))
		for _, user := range f.users {
			ids = append(ids, user.ID)
		}
		if len(ids) == 0 {
			return
		}
		var groupIDs []int64
		db.Model(&model.Group{}).Where("owner_id IN ?", ids).Pluck("id", &groupIDs)
		db.Where("from_id IN ? OR to_id IN ? OR group_id IN ?", ids, ids, groupIDs).Delete(&model.Message{})
		db.Where("from_id IN ? OR to_id IN ? OR group_id IN ?", ids, ids, groupIDs).Delete(&model.Conversation{})
		db.Where("user_id IN ? OR group_id IN ?", ids, groupIDs).Delete(&model.GroupApplication{})
		db.Where("user_id IN ? OR group_id IN ?", ids, groupIDs).Delete(&model.GroupMember{})
		db.Where("owner_id IN ?", ids).Delete(&model.Group{})
		db.Where("user_id IN ? OR friend_id IN ?", ids, ids).Delete(&model.Friend{})
		db.Where("low_id IN ? OR high_id IN ?", ids, ids).Delete(&model.PrivatePair{})
		db.Where("id IN ?", ids).Delete(&model.User{})
	})
	snowflake.Init(0, 1)
	ws := connect.NewWsConnServer()
	ws.Registry().Register(connect.CmdMsg, wsHandler.NewChatHandler(service.NewMessagingService(registry.NewMessageRepository(), registry.NewConversationRepository()), service.NewGroupService(registry.NewGroupRepository(), registry.NewGroupMemberRepository()), ws))
	f.wsServer = httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	t.Cleanup(func() { ws.Shutdown(context.Background()); f.wsServer.Close() })
	for i := 0; i < 4; i++ {
		u := model.User{Username: fmt.Sprintf("rel-%d-%d", time.Now().UnixNano(), i), Nickname: fmt.Sprintf("Person %d", i)}
		if err = db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		f.users = append(f.users, u)
		token, _ := util.GenerateToken(int(u.ID), u.Username, 1)
		f.tokens = append(f.tokens, token)
	}
	f.server = httptest.NewServer(NewRouter(registry.NewUserRepository(), registry.NewFriendRepository(), registry.NewGroupRepository(), registry.NewGroupMemberRepository(), registry.NewMessageRepository(), registry.NewConversationRepository(), nil, "builtin", "", 0, 0, nil, nil, WithRealtimeNotifier(func(command string, data any, users ...int64) {
		seen := map[int64]bool{}
		for _, id := range users {
			if !seen[id] {
				seen[id] = true
				ws.Send(fmt.Sprint(id), connect.NewRawMessage(command, "", data))
			}
		}
	})).Handler())
	t.Cleanup(f.server.Close)
	return f
}

func (f *relationshipFixture) notificationSocket(user int) *websocket.Conn {
	f.t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.wsServer.URL, "http")+"?token="+f.tokens[user], nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { conn.Close() })
	return conn
}
func (f *relationshipFixture) client(user int) *sdk.Client {
	f.t.Helper()
	client, err := sdk.NewClient(f.server.URL, sdk.WithToken(f.tokens[user]))
	if err != nil {
		f.t.Fatal(err)
	}
	return client
}
func expectEvent(t *testing.T, conn *websocket.Conn, command string) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var msg struct{ Cmd string }
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Cmd == command {
			return
		}
	}
}

func TestRelationshipSDKAndRealtimeNotifications(t *testing.T) {
	f := newRelationshipFixture(t)
	ctx := context.Background()
	a, b := f.client(0), f.client(1)
	bob := f.notificationSocket(1)
	if err := a.AddFriend(ctx, f.users[1].ID, ""); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, bob, "friend_changed")
	notices, err := b.ListFriendNotices(ctx)
	if err != nil || len(notices) != 1 || !notices[0].Unread {
		t.Fatalf("SDK notices=%+v err=%v", notices, err)
	}
	if err = b.ReadFriendNotice(ctx, f.users[0].ID); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, bob, "friend_changed")
	scope := fmt.Sprintf("private:%d", f.users[1].ID)
	if err = a.OpenConversation(ctx, scope); err != nil {
		t.Fatal(err)
	}
	conversations, err := a.ListConversations(ctx, 50, 0)
	if err != nil || len(conversations) != 1 || conversations[0].SendPolicy != "text_once" {
		t.Fatalf("SDK conversations=%+v err=%v", conversations, err)
	}
	group, err := a.CreateGroup(ctx, sdk.CreateGroupRequest{Name: "SDK review", MaxMembers: 2})
	if err != nil {
		t.Fatal(err)
	}
	application, err := b.ApplyToGroup(ctx, group.ID)
	if err != nil || application.Status != "pending" {
		t.Fatalf("SDK application=%+v err=%v", application, err)
	}
	expectEvent(t, bob, "group_changed")
	if err = b.JoinGroup(ctx, group.ID); err != sdk.ErrGroupApprovalPending {
		t.Fatalf("legacy SDK falsely reports membership: %v", err)
	}
	expectEvent(t, bob, "group_changed")
	applications, err := a.ListGroupApplications(ctx, group.ID, 50, 0)
	if err != nil || len(applications) != 1 || applications[0].ApplicantName != "Person 1" {
		t.Fatalf("SDK review queue=%+v err=%v", applications, err)
	}
	if _, err = a.ReviewGroupApplication(ctx, group.ID, application.ID, "approve", ""); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, bob, "group_changed")
	applications, err = b.ListMyGroupApplications(ctx, 50, 0)
	if err != nil || len(applications) != 1 || applications[0].Status != "approved" {
		t.Fatalf("SDK result=%+v err=%v", applications, err)
	}
}

func TestGroupApplicationRejectionInvitationTransferAndDissolution(t *testing.T) {
	f := newRelationshipFixture(t)
	raw := f.call(0, "POST", "/api/group/create", map[string]any{"name": "Lifecycle", "max_members": 3}, 200)
	var g model.Group
	json.Unmarshal(raw, &g)
	base := fmt.Sprintf("/api/group/%d", g.ID)
	raw = f.call(1, "POST", base+"/join", nil, 200)
	var first model.GroupApplication
	json.Unmarshal(raw, &first)
	f.call(0, "POST", fmt.Sprintf("%s/applications/%d/review", base, first.ID), map[string]any{"decision": "reject", "reason": "稍后再来"}, 200)
	raw = f.call(1, "POST", base+"/join", nil, 200)
	var next model.GroupApplication
	json.Unmarshal(raw, &next)
	if next.ID == first.ID {
		t.Fatal("reapplication overwrote rejected attempt")
	}
	f.call(0, "POST", base+"/invite", map[string]any{"user_id": f.users[1].ID}, 200)
	raw = f.call(1, "GET", "/api/group/applications/mine", nil, 200)
	var history []model.GroupApplication
	json.Unmarshal(raw, &history)
	if len(history) != 2 || history[0].Status != "approved" || history[1].Reason != "稍后再来" {
		t.Fatalf("invitation/history=%s", raw)
	}
	raw = f.call(2, "POST", base+"/join", nil, 200)
	var third model.GroupApplication
	json.Unmarshal(raw, &third)
	f.call(0, "POST", base+"/transfer", map[string]any{"user_id": f.users[1].ID}, 200)
	review := fmt.Sprintf("%s/applications/%d/review", base, third.ID)
	f.call(0, "POST", review, map[string]any{"decision": "approve"}, 403)
	f.call(1, "POST", review, map[string]any{"decision": "approve"}, 200)
	raw = f.call(3, "POST", base+"/join", nil, 200)
	var last model.GroupApplication
	json.Unmarshal(raw, &last)
	f.call(1, "POST", fmt.Sprintf("%s/applications/%d/review", base, last.ID), map[string]any{"decision": "approve"}, 409)
	raw = f.call(3, "GET", "/api/group/applications/mine", nil, 200)
	json.Unmarshal(raw, &history)
	if history[0].Status != "pending" {
		t.Fatalf("capacity failure consumed application: %s", raw)
	}
	f.call(1, "DELETE", base, nil, 200)
	raw = f.call(3, "GET", "/api/group/applications/mine", nil, 200)
	json.Unmarshal(raw, &history)
	if history[0].Status != "closed" {
		t.Fatalf("dissolution left pending request: %s", raw)
	}
}
func (f *relationshipFixture) call(user int, method, path string, body any, status int) json.RawMessage {
	f.t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, f.server.URL+path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+f.tokens[user])
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		f.t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.StatusCode, status, raw)
	}
	var envelope struct {
		Code int
		Data json.RawMessage
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		f.t.Fatal(err)
	}
	if status < 300 && envelope.Code != 0 {
		f.t.Fatalf("failed: %s", raw)
	}
	return envelope.Data
}
func TestFriendRelationshipsDoNotCreateConversations(t *testing.T) {
	f := newRelationshipFixture(t)
	a, b := f.users[0].ID, f.users[1].ID
	f.call(0, "POST", "/api/friend/add", map[string]any{"friend_id": b}, 200)
	for _, viewer := range []int{0, 1} {
		raw := f.call(viewer, "GET", "/api/conversation", nil, 200)
		var items []model.ConversationSummary
		json.Unmarshal(raw, &items)
		if len(items) != 0 {
			t.Fatalf("adding a friend created conversations: %s", raw)
		}
	}
	raw := f.call(1, "GET", "/api/friend/incoming", nil, 200)
	var notices []struct {
		UserID int64 `json:"user_id"`
		Unread bool  `json:"unread"`
	}
	json.Unmarshal(raw, &notices)
	if len(notices) != 1 || notices[0].UserID != a || !notices[0].Unread {
		t.Fatalf("missing incoming notice: %s", raw)
	}
	scope := fmt.Sprintf("/api/conversation/private:%d", b)
	f.call(0, "PATCH", scope, map[string]any{"draft": "keep draft"}, 200)
	f.call(0, "POST", scope+"/open", nil, 200)
	raw = f.call(0, "GET", "/api/conversation", nil, 200)
	var items []model.ConversationSummary
	json.Unmarshal(raw, &items)
	if len(items) != 1 || items[0].Draft != "keep draft" {
		t.Fatalf("opening overwrote draft: %s", raw)
	}
	f.call(1, "POST", fmt.Sprintf("/api/friend/%d/notice/read", a), nil, 200)
	raw = f.call(1, "GET", "/api/friend/incoming", nil, 200)
	json.Unmarshal(raw, &notices)
	if notices[0].Unread {
		t.Fatalf("notice unread survived acknowledgement: %s", raw)
	}
	f.call(0, "POST", "/api/friend/add", map[string]any{"friend_id": b}, 200)
}

func TestGroupApplicationsRequireOwnerApproval(t *testing.T) {
	f := newRelationshipFixture(t)
	raw := f.call(0, "POST", "/api/group/create", map[string]any{"name": "Review group", "max_members": 3}, 200)
	var group model.Group
	json.Unmarshal(raw, &group)
	base := fmt.Sprintf("/api/group/%d", group.ID)
	raw = f.call(1, "POST", base+"/join", nil, 200)
	var application struct {
		ID     int64
		Status string
	}
	json.Unmarshal(raw, &application)
	if application.Status != "pending" {
		t.Fatalf("join did not create pending application: %s", raw)
	}
	f.call(1, "GET", base+"/members", nil, 403)
	f.call(1, "GET", fmt.Sprintf("/api/conversation/group:%d/messages", group.ID), nil, 403)
	raw = f.call(1, "GET", "/api/group/list", nil, 200)
	var groups []model.Group
	json.Unmarshal(raw, &groups)
	if len(groups) != 0 {
		t.Fatalf("pending applicant became member: %s", raw)
	}
	duplicate := f.call(1, "POST", base+"/join", nil, 200)
	var same struct{ ID int64 }
	json.Unmarshal(duplicate, &same)
	if same.ID != application.ID {
		t.Fatalf("duplicate pending request: %s", duplicate)
	}
	review := fmt.Sprintf("%s/applications/%d/review", base, application.ID)
	f.call(2, "POST", review, map[string]any{"decision": "approve"}, 403)
	f.call(0, "POST", review, map[string]any{"decision": "approve"}, 200)
	f.call(0, "POST", review, map[string]any{"decision": "approve"}, 200)
	raw = f.call(1, "GET", "/api/group/applications/mine", nil, 200)
	var applications []struct{ Status string }
	json.Unmarshal(raw, &applications)
	if len(applications) != 1 || applications[0].Status != "approved" {
		t.Fatalf("applicant cannot see result: %s", raw)
	}
	f.call(1, "GET", base+"/members", nil, 200)
	raw = f.call(1, "GET", "/api/conversation", nil, 200)
	var conversations []model.ConversationSummary
	json.Unmarshal(raw, &conversations)
	if len(conversations) != 0 {
		t.Fatalf("approval created empty conversation: %s", raw)
	}
}
