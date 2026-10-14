package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseMessagePageDefaultsAndBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/api/conversation/private:7/messages", nil)
	limit, offset, err := parsePage(ctx)
	if err != nil || limit != 50 || offset != 0 {
		t.Fatalf("defaults = %d/%d/%v", limit, offset, err)
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/api/conversation/private:7/messages?limit=20&offset=40", nil)
	limit, offset, err = parsePage(ctx)
	if err != nil || limit != 20 || offset != 40 {
		t.Fatalf("page = %d/%d/%v", limit, offset, err)
	}
}

func TestParseMessagePageRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=-1", "?offset=nope"} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("GET", "/api/conversation/private:7/messages"+query, nil)
		if _, _, err := parsePage(ctx); err == nil {
			t.Fatalf("query %s was accepted", query)
		}
	}
}
