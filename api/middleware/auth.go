package middleware

import (
	"net/http"
	"strings"

	"gochat/pkg/logger"
	"gochat/pkg/util"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		logger.Info("Auth中间件", zap.String("path", c.Request.URL.Path), zap.String("auth", authHeader))
		if authHeader == "" {
			logger.Warn("未提供认证token")
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未提供认证token"})
			c.Abort()
			return
		}

		// Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("token格式错误", zap.String("auth", authHeader))
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token格式错误"})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := util.ParseToken(token)
		if err != nil {
			logger.Warn("Token解析失败", zap.String("error", err.Error()), zap.String("token", token))
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token无效"})
			c.Abort()
			return
		}

		logger.Info("Token解析成功", zap.Int("userID", claims.UserID))
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
