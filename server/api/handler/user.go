package handler

import (
	"net/http"
	"strconv"

	"gochat/logic/registry"
	"gochat/logic/service"
	"gochat/pkg/util"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userRepo registry.UserRepository) *UserHandler {
	return &UserHandler{
		userService: service.NewUserService(userRepo),
	}
}

// Register 注册
func (h *UserHandler) Register(c *gin.Context) {
	var req service.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	user, err := h.userService.Register(&req)
	if err != nil {
		if err == service.ErrUserExists {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "用户名已存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "注册成功",
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
		},
	})

}

// Login 登录
func (h *UserHandler) Login(c *gin.Context) {
	var req service.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	user, token, err := h.userService.Login(&req)
	if err != nil {
		if err == service.ErrUserNotExists {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "用户不存在"})
			return
		}
		if err == service.ErrPasswordIncorrect {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "密码错误"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "登录成功",
		"data": gin.H{
			"token": token,
			"user":  user,
		},
	})
}

// GetInfo 获取用户信息
func (h *UserHandler) GetInfo(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的用户ID"})
		return
	}

	user, err := h.userService.GetUserInfo(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": user,
	})
}

// UpdateProfile 更新资料
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	err := h.userService.UpdateProfile(userID, req.Nickname, req.Avatar)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "更新成功",
	})
}

// UpdateStatus 更新状态
func (h *UserHandler) UpdateStatus(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req struct {
		Status int8 `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	err := h.userService.UpdateStatus(userID, req.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "更新成功",
	})
}

// GenerateToken 生成测试token（仅开发环境）
func GenerateToken(userID int64, username string) (string, error) {
	return util.GenerateToken(int(userID), username, 48)
}
