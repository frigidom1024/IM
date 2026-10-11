package api

import (
	"context"
	"encoding/json"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/util"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestErrorsHaveUniformEnvelope(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, nil)
	for _, tc := range []struct {
		method, path string
		status, code int
	}{{"GET", "/missing", 404, 40400}, {"POST", "/livez", 405, 40500}, {"GET", "/api/user/info/1", 401, 40100}, {"POST", "/api/auth/login", 400, 40000}} {
		w := httptest.NewRecorder()
		r.Handler().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != tc.status || body["code"] != float64(tc.code) || body["message"] == nil {
			t.Fatalf("%s %s: status %d body %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if _, ok := body["data"]; !ok {
			t.Fatalf("missing data: %s", w.Body.String())
		}
	}
}

func TestMetricsUseRouteTemplatesAndTrackFailures(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, nil)
	for _, path := range []string{"/api/user/info/123?token=secret", "/unknown/123?token=secret", "/api/auth/login"} {
		w := httptest.NewRecorder()
		method := "GET"
		if path == "/api/auth/login" {
			method = "POST"
		}
		r.Handler().ServeHTTP(w, httptest.NewRequest(method, path, nil))
	}
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	for _, want := range []string{"gochat_http_requests_total{", `route="/api/user/info/:id"`, "gochat_http_request_duration_seconds_bucket", `gochat_auth_total{transport="http",result="failure"}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %s: %s", want, body)
		}
	}
	for _, secret := range []string{"secret", "/unknown/123", "/api/user/info/123"} {
		if strings.Contains(body, secret) {
			t.Fatalf("unbounded or secret metric label: %s", body)
		}
	}
	if w.Code != 200 {
		t.Fatalf("metrics status %d", w.Code)
	}
}

func TestReadinessDependencyFailureIsBoundedAndDoesNotAffectLiveness(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, func() bool { return true }, WithDependencyChecks(20*time.Millisecond, map[string]DependencyCheck{"mysql": func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, "minio": func(context.Context) error { return nil }, "redis": nil, "etcd": nil}))
	start := time.Now()
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 || time.Since(start) > time.Second {
		t.Fatalf("readiness status %d elapsed %s", w.Code, time.Since(start))
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != float64(50300) || !strings.Contains(w.Body.String(), `"redis":"disabled"`) || !strings.Contains(w.Body.String(), `"mysql":"unavailable"`) {
		t.Fatalf("readiness body %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/livez", nil))
	if w.Code != 200 {
		t.Fatalf("liveness status %d", w.Code)
	}
}

func TestHandlerPanicReturnsSafeErrorEnvelope(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, nil)
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/auth/login?token=private", strings.NewReader(`{"username":"tester","password":"private"}`)))
	if w.Code != 500 || !strings.Contains(w.Body.String(), `"code":50000`) || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("unsafe panic response: %d %s", w.Code, w.Body.String())
	}
}

type operationsUsers struct{ registry.UserRepository }

func (operationsUsers) GetByID(int64) (*model.User, error) {
	return &model.User{ID: 9223372036854775807, Username: "precision"}, nil
}
func TestResponseEnvelopePreservesSnowflakePrecision(t *testing.T) {
	util.Init("precision-test")
	token, _ := util.GenerateToken(1, "tester", 1)
	r := NewRouter(operationsUsers{}, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, nil)
	request := httptest.NewRequest("GET", "/api/user/info/1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":9223372036854775807`) {
		t.Fatalf("ID changed in response: %d %s", w.Code, w.Body.String())
	}
}
