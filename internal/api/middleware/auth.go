package middleware

import (
	"strings"

	"gochat/internal/pkg/errors"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/util"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("未提供认证token", zap.String("path", c.Request.URL.Path))
			errors.RespondError(c, errors.New(errors.CodeUnauthorized, "未提供认证token"))
			c.Abort()
			return
		}

		// Bearer token
		parts := strings.Fields(authHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			logger.Warn("token格式错误", zap.String("path", c.Request.URL.Path))
			errors.RespondError(c, errors.New(errors.CodeUnauthorized, "token格式错误"))
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := util.ParseToken(token)
		if err != nil {
			logger.Warn("Token解析失败", zap.String("error", err.Error()))
			errors.RespondError(c, errors.New(errors.CodeUnauthorized, "token无效"))
			c.Abort()
			return
		}

		logger.Debug("Token解析成功", zap.Int("userID", claims.UserID))
		// 将用户ID存入上下文
		c.Set("userID", int64(claims.UserID))
		c.Next()
	}
}

// GetUserID 获取当前用户ID
func GetUserID(c *gin.Context) int64 {
	userID, exists := c.Get("userID")
	if !exists {
		return 0
	}
	return userID.(int64)
}
