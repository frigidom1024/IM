package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"gochat/internal/connect"
	wsHandler "gochat/internal/connect/handler"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/mysql"
	"gochat/internal/pkg/snowflake"
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

// Runs against an isolated disposable database selected by the caller. Only
// fixture creation uses SQL; all assertions exercise authenticated HTTP or WS.
func TestGroupAdministrationHTTPAndWebSocket(t *testing.T) {
	dsn := os.Getenv("GOCHAT_GROUP_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOCHAT_GROUP_TEST_DSN to an isolated MySQL database")
	}
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := mysql.Client
	mysql.Client = db
	t.Cleanup(func() { mysql.Client = previous; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.User{}, &model.Group{}, &model.GroupMember{}, &model.GroupApplication{}, &model.Message{}, &model.UnreadCount{}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Init("error", "stdout"); err != nil {
		t.Fatal(err)
	}
	if err := snowflake.Init(0, 1); err != nil {
		t.Fatal(err)
	}
	users := make([]model.User, 4)
	tokens := make([]string, 4)
	prefix := fmt.Sprintf("group-%d", time.Now().UnixNano())
	for i := range users {
		users[i] = model.User{Username: fmt.Sprintf("%s-%d", prefix, i), Password: "fixture"}
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatal(err)
		}
		tokens[i], err = util.GenerateToken(int(users[i].ID), users[i].Username, 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, u := range users {
			db.Delete(&model.User{}, u.ID)
		}
	})
	groupRepo := registry.NewGroupRepository()
	memberRepo := registry.NewGroupMemberRepository()
	msgRepo := registry.NewMessageRepository()
	readRepo := registry.NewMessageReadRepository()
	router := NewRouter(registry.NewUserRepository(), registry.NewFriendRepository(), groupRepo, memberRepo, msgRepo, readRepo, nil, "local", "", 100, 5, nil, nil)
	httpServer := httptest.NewServer(router.engine)
	defer httpServer.Close()
	call := func(user int, method, path string, body any, want int) json.RawMessage {
		t.Helper()
		payload, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, httpServer.URL+path, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+tokens[user])
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s status %d want %d: %s", method, path, res.StatusCode, want, raw)
		}
		var envelope struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		if want == 200 && envelope.Code != 0 {
			t.Fatalf("operation failed: %s", raw)
		}
		return envelope.Data
	}
	data := call(0, "POST", "/api/group/create", map[string]any{"name": prefix, "max_members": 3}, 200)
	var group model.Group
	if err := json.Unmarshal(data, &group); err != nil {
		t.Fatal(err)
	}
	groupPath := fmt.Sprintf("/api/group/%d", group.ID)
	t.Cleanup(func() {
		db.Where("group_id = ?", group.ID).Delete(&model.Message{})
		db.Where("group_id = ?", group.ID).Delete(&model.UnreadCount{})
		db.Where("group_id = ?", group.ID).Delete(&model.GroupMember{})
		db.Delete(&model.Group{}, group.ID)
	})
	call(0, "DELETE", groupPath+"/leave", nil, 403)
	call(3, "GET", groupPath+"/members", nil, 403)
	call(0, "POST", groupPath+"/invite", map[string]any{"user_id": users[1].ID}, 200)
	call(1, "POST", groupPath+"/invite", map[string]any{"user_id": users[2].ID}, 403)
	call(0, "POST", groupPath+"/transfer", map[string]any{"user_id": users[3].ID}, 404)
	call(0, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[1].ID), map[string]any{"role": 1}, 200)
	call(1, "POST", groupPath+"/invite", map[string]any{"user_id": users[2].ID}, 200)
	var fullApplication model.GroupApplication
	json.Unmarshal(call(3, "POST", groupPath+"/join", nil, 200), &fullApplication)
	call(0, "POST", fmt.Sprintf("%s/applications/%d/review", groupPath, fullApplication.ID), map[string]string{"decision": "approve"}, 409)
	call(1, "PUT", groupPath+"/announcement", map[string]any{"announcement": "hello team"}, 200)
	call(1, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[0].ID), map[string]any{"muted_until": time.Now().Add(time.Hour).UTC()}, 403)
	call(1, "DELETE", fmt.Sprintf("%s/members/%d", groupPath, users[0].ID), nil, 403)
	call(1, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[2].ID), map[string]any{"role": 1}, 403)
	call(2, "PUT", groupPath+"/announcement", map[string]any{"announcement": "forged"}, 403)
	call(2, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[2].ID), map[string]any{"nickname": "test nickname"}, 200)
	call(2, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[1].ID), map[string]any{"role": 2}, 400)
	var members []model.GroupMember
	if err := json.Unmarshal(call(0, "GET", groupPath+"/members?limit=1&offset=1", nil, 200), &members); err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].Role != model.RoleAdmin {
		t.Fatalf("pagination or admin role incorrect: %+v", members)
	}
	var found []model.Group
	if err := json.Unmarshal(call(2, "GET", "/api/group/search?q="+prefix, nil, 200), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Announcement != "hello team" {
		t.Fatalf("search/announcement incorrect: %+v", found)
	}

	ws := connect.NewWsConnServer()
	msgService := service.NewMessageService(msgRepo, readRepo)
	ws.Registry().Register(connect.CmdMsg, wsHandler.NewChatHandler(msgService, service.NewGroupService(groupRepo, memberRepo), ws))
	ws.Registry().Register(connect.CmdMsgRead, wsHandler.NewMsgReadHandler(msgService, nil, ws))
	wsServer := httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	defer wsServer.Close()
	defer ws.Shutdown(context.Background())
	wsCall := func(user int, cmd, seq string, body any, wantCmd string) int64 {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(wsServer.URL, "http")+"?token="+tokens[user], nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(map[string]any{"cmd": cmd, "seq": seq, "data": body}); err != nil {
			t.Fatal(err)
		}
		var reply struct {
			Cmd  string `json:"cmd"`
			Data struct {
				MessageID int64 `json:"message_id"`
			} `json:"data"`
		}
		if err := conn.ReadJSON(&reply); err != nil {
			t.Fatal(err)
		}
		if reply.Cmd != wantCmd {
			t.Fatalf("WS command %s returned %s want %s", cmd, reply.Cmd, wantCmd)
		}
		return reply.Data.MessageID
	}
	send := func(user int, seq string, want string) int64 {
		return wsCall(user, "msg", seq, map[string]any{"from_id": users[user].ID, "group_id": group.ID, "type": "text", "content": seq}, want)
	}
	send(3, "nonmember", "error")
	call(1, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[2].ID), map[string]any{"muted_until": time.Now().Add(time.Hour).UTC()}, 200)
	send(2, "muted", "error")
	call(1, "PATCH", fmt.Sprintf("%s/members/%d", groupPath, users[2].ID), map[string]any{"unmute": true}, 200)
	messageID := send(2, "allowed", "msg_reply")
	call(3, "GET", fmt.Sprintf("/api/message/group/%d", group.ID), nil, 403)
	receiptPath := fmt.Sprintf("%s/messages/%d/receipts", groupPath, messageID)
	call(3, "GET", receiptPath, nil, 403)
	wsCall(3, "msg_read", "invalid-read", map[string]any{"message_id": fmt.Sprint(messageID)}, "error")
	wsCall(1, "msg_read", "read", map[string]any{"message_id": fmt.Sprint(messageID)}, "msg_read_reply")
	if err := json.Unmarshal(call(0, "GET", receiptPath, nil, 200), &members); err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].UserID != users[1].ID {
		t.Fatalf("receipts incorrect: %+v", members)
	}
	call(2, "POST", "/api/message/read", map[string]any{"group_id": group.ID, "message_id": messageID}, 200)
	laterMessageID := send(2, "later-message", "msg_reply")
	call(1, "POST", "/api/message/read", map[string]any{"group_id": group.ID, "message_id": messageID}, 200)
	var unread struct {
		Count int64 `json:"count"`
	}
	if err := json.Unmarshal(call(1, "GET", "/api/message/unread", nil, 200), &unread); err != nil {
		t.Fatal(err)
	}
	if unread.Count != 1 {
		t.Fatalf("old cursor cleared later unread: %+v", unread)
	}
	call(1, "POST", "/api/message/read", map[string]any{"group_id": group.ID, "message_id": laterMessageID}, 200)
	call(1, "POST", "/api/message/read", map[string]any{"group_id": group.ID, "message_id": messageID}, 200)
	if err := json.Unmarshal(call(1, "GET", "/api/message/unread", nil, 200), &unread); err != nil {
		t.Fatal(err)
	}
	if unread.Count != 0 {
		t.Fatalf("read cursor moved backwards: %+v", unread)
	}
	call(0, "POST", groupPath+"/transfer", map[string]any{"user_id": users[1].ID}, 200)
	if err := json.Unmarshal(call(1, "GET", groupPath+"/members", nil, 200), &members); err != nil {
		t.Fatal(err)
	}
	if members[0].UserID != users[1].ID || members[0].Role != model.RoleOwner {
		t.Fatalf("transfer roles incorrect: %+v", members)
	}
	if err := json.Unmarshal(call(1, "GET", "/api/group/list", nil, 200), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].OwnerID != users[1].ID {
		t.Fatalf("group owner and roles diverged: %+v", found)
	}
	call(0, "DELETE", groupPath, nil, 403)
	call(1, "DELETE", fmt.Sprintf("%s/members/%d", groupPath, users[2].ID), nil, 200)
	send(2, "removed", "error")
	call(2, "GET", receiptPath, nil, 403)
	call(1, "DELETE", groupPath, nil, 200)
	call(1, "GET", groupPath+"/members", nil, 403)
	send(1, "dissolved", "error")
	if err := json.Unmarshal(call(0, "GET", "/api/group/search?q="+prefix, nil, 200), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("dissolved group still discoverable: %+v", found)
	}

	data = call(0, "POST", "/api/group/create", map[string]any{"name": prefix + "-capacity", "max_members": 2}, 200)
	var limited model.Group
	if err := json.Unmarshal(data, &limited); err != nil {
		t.Fatal(err)
	}
	limitedPath := fmt.Sprintf("/api/group/%d", limited.ID)
	t.Cleanup(func() {
		db.Where("group_id = ?", limited.ID).Delete(&model.GroupMember{})
		db.Delete(&model.Group{}, limited.ID)
	})
	statuses := make(chan int, 3)
	for _, user := range []int{1, 2, 3} {
		var application model.GroupApplication
		json.Unmarshal(call(user, "POST", limitedPath+"/join", nil, 200), &application)
		go func(applicationID int64) {
			req, _ := http.NewRequest("POST", fmt.Sprintf("%s%s/applications/%d/review", httpServer.URL, limitedPath, applicationID), strings.NewReader(`{"decision":"approve"}`))
			req.Header.Set("Authorization", "Bearer "+tokens[0])
			req.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			statuses <- response.StatusCode
		}(application.ID)
	}
	success, full := 0, 0
	for range 3 {
		switch status := <-statuses; status {
		case 200:
			success++
		case 409:
			full++
		default:
			t.Fatalf("concurrent join status = %d", status)
		}
	}
	if success != 1 || full != 2 {
		t.Fatalf("capacity race results = %d successes/%d full", success, full)
	}
	if err := json.Unmarshal(call(0, "GET", limitedPath+"/members", nil, 200), &members); err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("concurrent joins exceeded capacity: %+v", members)
	}
	call(0, "DELETE", limitedPath, nil, 200)
}
