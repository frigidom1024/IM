package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroupCreateRejectsInvalidCapacityOverHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userID", int64(1)) })
	router.POST("/api/group/create", NewGroupHandler(nil, nil).Create)
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/group/create", strings.NewReader(`{"name":"team","max_members":-1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != 400 {
		t.Fatalf("invalid capacity status = %d, body = %s", response.Code, response.Body.String())
	}
}
