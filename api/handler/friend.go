package handler

import (
	"net/http"
	"strconv"

	"gochat/logic/registry"
	"gochat/logic/service"

	"github.com/gin-gonic/gin"
)

// FriendHandler 好友处理器
type FriendHandler struct {
	friendService *service.FriendService
}

// NewFriendHandler 创建好友处理器
func NewFriendHandler(friendRepo registry.FriendRepository) *FriendHandler {
	return &FriendHandler{
		friendService: service.NewFriendService(friendRepo),
	}
}

// GetList 获取好友列表
func (h *FriendHandler) GetList(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	friends, err := h.friendService.GetList(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data":  friends,
	})
}

// Add 添加好友
func (h *FriendHandler) Add(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req struct {
		FriendID int64  `json:"friend_id"`
		Remark   string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	if req.FriendID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "friend_id不能为空"})
		return
	}

	err := h.friendService.Add(userID, req.FriendID, req.Remark)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "添加成功",
	})
}

// Remove 删除好友
func (h *FriendHandler) Remove(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	friendIDStr := c.Param("friendId")
	friendID, err := strconv.ParseInt(friendIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的friendId"})
		return
	}

	err = h.friendService.Remove(userID, friendID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "删除成功",
	})
}
