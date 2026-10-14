package connect_test

import (
	"errors"
	"fmt"
	"gochat/internal/connect/handler"
	"gochat/internal/logic/model"
	"gochat/internal/pkg/snowflake"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gochat/internal/connect"
	"gochat/internal/pkg/telemetry"
	"gochat/internal/pkg/util"
)

func TestWebSocketMetricsReflectAuthenticationAndConnectionLifecycle(t *testing.T) {
	metrics := telemetry.New()
	ws := connect.NewWsConnServer(metrics)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", ws.HandleWebSocket)
	mux.Handle("/metrics", metrics)
	server := httptest.NewServer(mux)
	defer server.Close()
	util.Init("ws-metrics-test-secret")
	token, err := util.GenerateToken(42, "tester", 1)
	if err != nil {
		t.Fatal(err)
	}
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	if conn, _, err := websocket.DefaultDialer.Dial(url, nil); err == nil {
		conn.Close()
		t.Fatal("unauthenticated connection accepted")
	}
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatal(err)
	}
	scrape := func() string {
		w := httptest.NewRecorder()
		metrics.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		return w.Body.String()
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(scrape(), "gochat_ws_connections 1") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for _, want := range []string{"gochat_ws_connections 1", "gochat_ws_users 1", `gochat_auth_total{transport="ws",result="success"} 1`, `gochat_auth_total{transport="ws",result="failure"} 1`} {
		if !strings.Contains(scrape(), want) {
			t.Fatalf("missing %s: %s", want, scrape())
		}
	}
	_ = conn.Close()
	deadline = time.Now().Add(time.Second)
	for !strings.Contains(scrape(), "gochat_ws_connections 0") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(scrape(), "gochat_ws_connections 0") || !strings.Contains(scrape(), "gochat_ws_users 0") {
		t.Fatalf("connections not removed: %s", scrape())
	}
}

type messageStore struct{}

func (messageStore) Send(m *model.Message) ([]int64, error) {
	if m.Content == "fail" {
		return nil, errors.New("storage unavailable")
	}
	return []int64{m.ToID}, nil
}
func (messageStore) GetByClientSeq(int64, string) (*model.Message, error) { return nil, nil }
func (messageStore) GetByID(int64) (*model.Message, error)                { return nil, nil }

func TestWebSocketMetricsCountPersistenceOutcomes(t *testing.T) {
	metric := func(result string) int {
		w := httptest.NewRecorder()
		telemetry.Default.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		prefix := fmt.Sprintf("gochat_messages_total{result=%q} ", result)
		for _, line := range strings.Split(w.Body.String(), "\n") {
			if strings.HasPrefix(line, prefix) {
				value, _ := strconv.Atoi(strings.TrimPrefix(line, prefix))
				return value
			}
		}
		t.Fatalf("missing metric %s", result)
		return 0
	}
	persistedBefore, failedBefore := metric("persisted"), metric("persistence_failure")

	if err := snowflake.Init(1, 1); err != nil {
		t.Fatal(err)
	}
	ws := connect.NewWsConnServer()
	ws.Registry().Register(connect.CmdMsg, handler.NewChatHandler(messageStore{}, nil, ws))
	server := httptest.NewServer(http.HandlerFunc(ws.HandleWebSocket))
	defer server.Close()
	util.Init("ws-persistence-test")
	token, _ := util.GenerateToken(42, "tester", 1)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, content := range []string{"ok", "fail"} {
		data, _ := connect.NewRawMessage(connect.CmdMsg, content, &connect.ChatMsgData{ConversationID: "private:8", Type: model.MsgTypeText, Content: content}).ToJSON()
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatal(err)
		}
		conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	telemetry.Default.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, want := range []string{fmt.Sprintf(`gochat_messages_total{result="persisted"} %d`, persistedBefore+1), fmt.Sprintf(`gochat_messages_total{result="persistence_failure"} %d`, failedBefore+1)} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing persistence result %s: %s", want, w.Body.String())
		}
	}
}
