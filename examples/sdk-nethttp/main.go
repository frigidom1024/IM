// Run with GOCHAT_EXAMPLE_TOKEN set to a development token.
package main

import (
	"context"
	"errors"
	"gochat/sdk"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	token := os.Getenv("GOCHAT_EXAMPLE_TOKEN")
	if token == "" {
		log.Fatal("set GOCHAT_EXAMPLE_TOKEN")
	}
	resolver := sdk.UserResolverFunc(func(ctx context.Context, presented string) (sdk.ExternalUser, error) {
		if presented != token {
			return sdk.ExternalUser{}, errors.New("unauthorized")
		}
		return sdk.ExternalUser{ID: "demo-user", Name: "Demo user"}, nil
	})
	mux := http.NewServeMux()
	mux.Handle("/api/user/info", sdk.NewUserInfoHandler(resolver))
	server := &http.Server{Addr: "127.0.0.1:8090", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
