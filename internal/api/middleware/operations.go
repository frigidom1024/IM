package middleware

import (
	"bytes"
	"encoding/json"
	"go.uber.org/zap"
	"gochat/internal/pkg/telemetry"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gochat/internal/pkg/errors"
	"gochat/internal/pkg/logger"
)

// Envelopes normalizes legacy JSON responses at write time without buffering
// downloads or changing non-JSON streams. Gin renders each JSON document in one Write.
func Envelopes() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &envelopeWriter{ResponseWriter: c.Writer, status: 200}
		c.Next()
	}
}

type envelopeWriter struct {
	gin.ResponseWriter
	status int
}

func (w *envelopeWriter) WriteHeader(status int) {
	if !w.Written() {
		w.status = status
	}
}
func (w *envelopeWriter) Status() int { return w.status }
func (w *envelopeWriter) WriteHeaderNow() {
	w.ResponseWriter.WriteHeader(w.status)
	w.ResponseWriter.WriteHeaderNow()
}
func (w *envelopeWriter) Write(data []byte) (int, error) {
	originalLen := len(data)
	if w.Header().Get("Content-Disposition") == "" && strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		var body map[string]any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if decoder.Decode(&body) == nil && body != nil {
			if number, ok := body["code"].(json.Number); ok {
				code, _ := number.Int64()
				if code == 1 {
					body["code"] = errors.CodeServerError
					body["message"] = "internal server error"
					w.status = 500
				} else if code >= 400 && code < 600 {
					body["code"] = int(code) * 100
				}
				if _, ok := body["message"]; !ok {
					body["message"] = "ok"
				}
				if _, ok := body["data"]; !ok {
					body["data"] = nil
				}
				if normalized, err := json.Marshal(body); err == nil {
					data = normalized
					w.Header().Del("Content-Length")
				}
			}
		}
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, err := w.ResponseWriter.Write(data)
	return originalLen, err
}
func (w *envelopeWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *envelopeWriter) Flush()                            { w.WriteHeaderNow(); w.ResponseWriter.Flush() }

// Recovery intentionally logs no request headers, query string, body or panic
// value: each can contain credentials or private message content.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				logger.Error("HTTP handler panic")
				if !c.Writer.Written() {
					c.Abort()
					c.JSON(http.StatusInternalServerError, gin.H{"code": errors.CodeServerError, "message": "internal server error", "data": nil})
				}
			}
		}()
		c.Next()
	}
}

// Metrics uses Gin's registered route template, never the raw URL or query.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		route := c.FullPath()
		status := c.Writer.Status()
		telemetry.Default.ObserveHTTP(c.Request.Method, route, status, time.Since(started).Seconds())
		if strings.HasPrefix(route, "/api/auth") {
			telemetry.Default.Auth("http", status < 400)
		}
		if strings.HasPrefix(route, "/api/upload") && status >= 400 {
			telemetry.Default.UploadFailure()
		}
		logger.Info("HTTP request", zap.String("method", safeMethod(c.Request.Method)), zap.String("route", route), zap.Int("status", status), zap.Duration("duration", time.Since(started)))
	}
}
func safeMethod(method string) string {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return method
	}
	return "OTHER"
}
