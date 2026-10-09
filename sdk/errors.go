package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIError is returned when GoChat rejects a request or responds with a
// non-success HTTP status.
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
}

func (e *APIError) Error() string {
	if e.HTTPStatus != 0 && e.Code != 0 {
		return fmt.Sprintf("gochat sdk: http=%d code=%d: %s", e.HTTPStatus, e.Code, e.Message)
	}
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("gochat sdk: http=%d: %s", e.HTTPStatus, e.Message)
	}
	return fmt.Sprintf("gochat sdk: code=%d: %s", e.Code, e.Message)
}

// IsUnauthorized reports whether err represents an authentication failure.
func IsUnauthorized(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && (apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.Code == 401 || apiErr.Code == 40100)
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}
