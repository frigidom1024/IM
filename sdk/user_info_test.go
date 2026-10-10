package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserInfoHandler(t *testing.T) {
	var receivedToken string
	handler := NewUserInfoHandler(UserResolverFunc(func(_ context.Context, token string) (ExternalUser, error) {
		receivedToken = token
		if token != "external-token" {
			return ExternalUser{}, errors.New("invalid token")
		}
		return ExternalUser{ID: "42", Name: "Alice"}, nil
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer external-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if receivedToken != "external-token" {
		t.Fatalf("received token = %q", receivedToken)
	}

	request, err = http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", response.StatusCode)
	}
}
