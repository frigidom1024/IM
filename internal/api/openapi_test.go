package api

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	apidocs "gochat/docs/api"
)

func TestOpenAPICoversRegisteredRoutesAndSchemas(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(apidocs.OpenAPI, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Fatal("OpenAPI version must be 3.0.3")
	}
	paths := doc["paths"].(map[string]any)
	operationIDs := map[string]bool{}
	for path, item := range paths {
		for method, value := range item.(map[string]any) {
			operation := value.(map[string]any)
			id, ok := operation["operationId"].(string)
			if !ok || id == "" || operationIDs[id] {
				t.Errorf("missing or duplicate operation ID %s %s", method, path)
			}
			operationIDs[id] = true
		}
	}

	params := regexp.MustCompile(`:([A-Za-z0-9_]+)`)
	for _, mode := range []string{"builtin", "external"} {
		r := NewRouter(nil, nil, nil, nil, nil, nil, nil, mode, "http://auth.test", 0, 0, nil, nil)
		registered := false
		for _, route := range r.engine.Routes() {
			if route.Path == "/openapi.json" {
				registered = true
			}
		}
		if !registered {
			registerOpenAPI(r.engine)
		}
		for _, route := range r.engine.Routes() {
			path := params.ReplaceAllString(route.Path, "{$1}")
			item, ok := paths[path].(map[string]any)
			if !ok {
				t.Errorf("undocumented path %s", path)
				continue
			}
			operation, ok := item[strings.ToLower(route.Method)].(map[string]any)
			if !ok {
				t.Errorf("undocumented operation %s %s", route.Method, path)
				continue
			}
			if operation["summary"] == nil || operation["operationId"] == nil || operation["responses"] == nil {
				t.Errorf("incomplete operation %s %s", route.Method, path)
			}
			declared := map[string]bool{}
			if parameters, ok := operation["parameters"].([]any); ok {
				for _, p := range parameters {
					parameter := p.(map[string]any)
					if parameter["in"] == "path" {
						name, _ := parameter["name"].(string)
						declared[name] = parameter["required"] == true
						if parameter["schema"] == nil {
							t.Errorf("missing path schema %s", name)
						}
					}
				}
			}
			for _, match := range params.FindAllStringSubmatch(route.Path, -1) {
				if !declared[match[1]] {
					t.Errorf("missing required path parameter %s %s", path, match[1])
				}
			}
			security, ok := operation["security"].([]any)
			protected := strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/auth")
			if !ok || (protected && len(security) == 0) || (!protected && len(security) != 0) {
				t.Errorf("incorrect authentication for %s %s", route.Method, path)
			}
			for status, response := range operation["responses"].(map[string]any) {
				if status == "default" || (strings.HasPrefix(status, "4") || strings.HasPrefix(status, "5")) && status != "416" {
					entry := response.(map[string]any)
					if reference, ok := entry["$ref"].(string); ok {
						entry = doc["components"].(map[string]any)["responses"].(map[string]any)[strings.TrimPrefix(reference, "#/components/responses/")].(map[string]any)
					}
					schema := entry["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
					if path == "/readyz" && status == "503" {
						if schema["$ref"] != "#/components/schemas/Readiness" {
							t.Error("readiness schema missing")
						}
					} else if schema["$ref"] != "#/components/schemas/ErrorEnvelope" {
						t.Errorf("missing error envelope %s %s", path, status)
					}
				}
			}

		}
		w := httptest.NewRecorder()
		r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
		var served map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &served) != nil || served["openapi"] != "3.0.3" {
			t.Fatalf("OpenAPI endpoint: %d %s", w.Code, w.Body.String())
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if reference, ok := v["$ref"].(string); ok {
				if !strings.HasPrefix(reference, "#/") {
					t.Errorf("external ref %s", reference)
				} else {
					var target any = doc
					for _, segment := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
						m, ok := target.(map[string]any)
						if !ok {
							t.Errorf("invalid ref %s", reference)
							break
						}
						target, ok = m[segment]
						if !ok {
							t.Errorf("missing ref %s", reference)
							break
						}
					}
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(doc)
	var validateSchema func(any)
	validateSchema = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if kind, ok := v["type"].(string); ok {
				switch kind {
				case "object", "array", "string", "integer", "number", "boolean":
				default:
					t.Errorf("invalid OpenAPI schema type %s", kind)
				}
				if kind == "array" && v["items"] == nil {
					t.Error("array schema missing items")
				}
			}
			if properties, ok := v["properties"].(map[string]any); ok {
				if required, ok := v["required"].([]any); ok {
					for _, name := range required {
						field, ok := name.(string)
						if !ok || properties[field] == nil {
							t.Errorf("invalid required schema field %v", name)
						}
					}
				}
			}
			for _, child := range v {
				validateSchema(child)
			}
		case []any:
			for _, child := range v {
				validateSchema(child)
			}
		}
	}
	validateSchema(doc["components"].(map[string]any)["schemas"])

	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	envelope := schemas["ErrorEnvelope"].(map[string]any)
	properties := envelope["properties"].(map[string]any)
	required := map[string]bool{}
	for _, name := range envelope["required"].([]any) {
		required[name.(string)] = true
	}
	for _, field := range []string{"code", "message", "data"} {
		if !required[field] {
			t.Errorf("error envelope field %s must be required", field)
		}
		if properties[field] == nil {
			t.Errorf("error envelope missing %s", field)
		}
	}
}
