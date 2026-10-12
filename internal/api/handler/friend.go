package handler

import (
	"errors"
	apierrors "gochat/internal/pkg/errors"
	"net/http"
	"strconv"

	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"

	"github.com/gin-gonic/gin"
)

// FriendHandler 好友处理器
type FriendHandler struct {
	friendService *service.FriendService
	notify        Notify
}

// NewFriendHandler 创建好友处理器
func NewFriendHandler(friendRepo registry.FriendRepository, notifications ...Notify) *FriendHandler {
	h := &FriendHandler{
		friendService: service.NewFriendService(friendRepo),
	}
	if len(notifications) > 0 {
		h.notify = notifications[0]
	}
	return h
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
		"data": friends,
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
		friendError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "添加成功",
	})
	h.notify.send("friend_changed", gin.H{"user_id": userID, "peer_id": req.FriendID}, userID, req.FriendID)
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
		friendError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "删除成功",
	})
	h.notify.send("friend_changed", gin.H{"user_id": userID, "peer_id": friendID}, userID, friendID)
}

func friendError(c *gin.Context, err error) {
	var business *apierrors.Error
	if errors.As(err, &business) {
		apierrors.RespondError(c, business)
	} else {
		apierrors.RespondError(c, apierrors.New(apierrors.CodeServerError, "好友操作失败"))
	}
}
func (h *FriendHandler) Incoming(c *gin.Context) {
	items, err := h.friendService.Incoming(c.GetInt64("userID"))
	if err != nil {
		friendError(c, err)
		return
	}
	apierrors.RespondOK(c, items)
}
func (h *FriendHandler) ReadNotice(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("friendId"), 10, 64)
	if err != nil || id <= 0 {
		apierrors.RespondBadRequest(c, "无效的用户ID")
		return
	}
	if err = h.friendService.ReadNotice(c.GetInt64("userID"), id); err != nil {
		friendError(c, err)
		return
	}
	apierrors.RespondOK(c, nil)
	h.notify.send("friend_changed", gin.H{"user_id": c.GetInt64("userID")}, c.GetInt64("userID"))
}
