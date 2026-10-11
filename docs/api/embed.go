// Package apidocs embeds the canonical HTTP API contract for offline serving.
package apidocs

import _ "embed"

// OpenAPI is the checked-in OpenAPI 3.0.3 JSON document.
//
//go:embed openapi.json
var OpenAPI []byte
