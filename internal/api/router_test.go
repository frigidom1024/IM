package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpointsReflectLivenessAndReadiness(t *testing.T) {
	ready := false
	router := NewRouter(nil, nil, nil, nil, nil, nil, nil, "builtin", "", 0, 0, nil, func() bool { return ready })

	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	response := httptest.NewRecorder()
	router.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("livez status = %d, want %d", response.Code, http.StatusOK)
	}

	request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response = httptest.NewRecorder()
	router.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz before startup = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}

	ready = true
	request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response = httptest.NewRecorder()
	router.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("readyz after startup = %d, want %d", response.Code, http.StatusOK)
	}
}
