package errors

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"gochat/pkg/logger"
)

// httpStatusMap 错误码 → HTTP 状态码映射
var httpStatusMap = map[int]int{
	CodeSuccess:        http.StatusOK,
	CodeBadRequest:     http.StatusBadRequest,
	CodeInvalidParam:   http.StatusBadRequest,
	CodeUnauthorized:   http.StatusUnauthorized,
	CodeServerError:    http.StatusInternalServerError,
	CodeUploadTooLarge: http.StatusBadRequest,
	CodeUploadBadMIME:  http.StatusBadRequest,
	CodeUploadBadType:  http.StatusBadRequest,
	CodeUploadMissType: http.StatusBadRequest,
	CodeUploadReadFail: http.StatusInternalServerError,
	CodeUploadFail:     http.StatusInternalServerError,
}

// resolveHTTPStatus 根据错误码确定 HTTP 状态码
func resolveHTTPStatus(code int) int {
	if status, ok := httpStatusMap[code]; ok {
		return status
	}
	// 未显式映射的错误码: 4xxxx → 400, 5xxxx → 500
	if code >= 50000 {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

// RespondOK 成功响应
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": CodeSuccess,
		"data": data,
	})
}

// RespondOKWithMsg 成功响应（带消息）
func RespondOKWithMsg(c *gin.Context, data any, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code":    CodeSuccess,
		"message": message,
		"data":    data,
	})
}

// RespondError 统一错误响应
func RespondError(c *gin.Context, err *Error) {
	status := resolveHTTPStatus(err.Code)

	// 服务端错误记录日志
	if status == http.StatusInternalServerError {
		logger.Error("服务器错误", append(err.LogFields(), zap.String("path", c.Request.URL.Path))...)
	}

	c.JSON(status, gin.H{
		"code":    err.Code,
		"message": err.Message,
	})
}

// RespondBadRequest 快速返回参数错误
func RespondBadRequest(c *gin.Context, message string) {
	RespondError(c, New(CodeBadRequest, message))
}
