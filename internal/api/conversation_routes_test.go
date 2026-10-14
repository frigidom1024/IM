package api

import (
	"gochat/internal/pkg/util"
	"net/http/httptest"
	"testing"
)

func TestLegacyMessageAndReceiptRoutesAreRemoved(t *testing.T) {
	util.Init("removed-conversation-routes-test")
	token, err := util.GenerateToken(1, "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, nil)
	for _, test := range []struct{ method, path string }{
		{"POST", "/api/message/read"}, {"GET", "/api/message/unread"},
		{"GET", "/api/message/private/2"}, {"GET", "/api/message/group/7"},
		{"GET", "/api/group/7/messages/9/receipts"},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		r.Handler().ServeHTTP(response, request)
		if response.Code != 404 {
			t.Fatalf("%s %s: %d", test.method, test.path, response.Code)
		}
	}
}
