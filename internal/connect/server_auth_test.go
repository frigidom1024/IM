package connect

import "testing"

func TestNormalizeWebSocketToken(t *testing.T) {
	cases := map[string]string{
		"token":            "token",
		"Bearer token":     "token",
		"bearer token":     "token",
		"  Bearer token  ": "token",
		"Bearer":           "",
		"":                 "",
	}
	for input, want := range cases {
		if got := normalizeWebSocketToken(input); got != want {
			t.Errorf("normalizeWebSocketToken(%q) = %q, want %q", input, got, want)
		}
	}
}
