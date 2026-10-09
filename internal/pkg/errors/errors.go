package errors

import (
	"fmt"

	"go.uber.org/zap"
)

// Error 统一业务错误
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"` // 内部原始错误，不暴露给客户端
}

// New 创建业务错误
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// NewWithError 携带内部错误创建业务错误（用于日志记录原始错误）
func NewWithError(code int, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("error code: %d", e.Code)
}

// Unwrap 支持 errors.Is/As 链式查找
func (e *Error) Unwrap() error { return e.Err }

// LogFields 返回结构化日志字段
func (e *Error) LogFields() []zap.Field {
	fields := []zap.Field{zap.Int("err_code", e.Code), zap.String("err_msg", e.Message)}
	if e.Err != nil {
		fields = append(fields, zap.Error(e.Err))
	}
	return fields
}
