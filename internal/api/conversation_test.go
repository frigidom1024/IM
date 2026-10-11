package api

import (
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/util"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConversationPreferenceRejectsInvalidScope(t *testing.T) {
	if err := logger.Init("error", "stdout"); err != nil {
		t.Fatal(err)
	}
	util.Init("p1-test-secret")
	token, err := util.GenerateToken(1, "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/conversation/private:0", strings.NewReader(`{"pinned":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
