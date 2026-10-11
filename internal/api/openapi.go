package api

import (
	"github.com/gin-gonic/gin"
	apidocs "gochat/docs/api"
	"net/http"
)

// registerOpenAPI exposes the canonical embedded contract without external assets.
func registerOpenAPI(engine *gin.Engine) {
	engine.GET("/openapi.json", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/json; charset=utf-8", apidocs.OpenAPI)
	})
}
