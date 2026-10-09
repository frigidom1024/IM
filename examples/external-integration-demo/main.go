package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

type externalUser struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

var users = map[string]externalUser{
	"demo-alice": {ID: "external-alice", Name: "Alice"},
	"demo-bob":   {ID: "external-bob", Name: "Bob"},
}

//go:embed public/index.html
var indexHTML []byte

func main() {
	mode := os.Getenv("MODE")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	mux := http.NewServeMux()
	if mode == "s3" {
		registerS3Stub(mux)
	} else {
		registerExternalApp(mux)
	}

	log.Printf("integration demo %s listening on :%s", modeName(mode), port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func modeName(mode string) string {
	if mode == "s3" {
		return "S3 stub"
	}
	return "external app"
}

func registerExternalApp(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "external-integration-demo",
		})
	})

	mux.HandleFunc("/api/demo/users", func(w http.ResponseWriter, r *http.Request) {
		result := make([]externalUser, 0, len(users))
		for _, user := range users {
			result = append(result, user)
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/api/user/info", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		user, ok := users[strings.TrimSpace(token)]
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid external token"})
			return
		}
		writeJSON(w, http.StatusOK, user)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML)
	})
}

func registerS3Stub(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "s3-stub"})
	})

	// The auth/chat E2E does not exercise file upload. This tiny S3-compatible
	// stub accepts bucket setup requests made by the MinIO client at startup.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "integration-s3-stub")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}