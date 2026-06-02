package handler

import (
	"net/http"
	"strconv"

	"gochat/logic/registry"
	"gochat/logic/service"

	"github.com/gin-gonic/gin"
)

const (
	RoleOwner  = 1
	RoleAdmin  = 2
	RoleMember = 3
)

// GroupHandler 群组处理器
type GroupHandler struct {
	groupService *service.GroupService
}

// NewGroupHandler 创建群组处理器
func NewGroupHandler(
	groupRepo registry.GroupRepository,
	groupMemberRepo registry.GroupMemberRepository,
) *GroupHandler {
	return &GroupHandler{
		groupService: service.NewGroupService(groupRepo, groupMemberRepo),
	}
}

// Create 创建群组
func (h *GroupHandler) Create(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req struct {
		Name      string `json:"name"`
		Avatar    string `json:"avatar"`
		MaxMembers int    `json:"max_members"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "群组名称不能为空"})
		return
	}

	maxMembers := req.MaxMembers
	if maxMembers == 0 {
		maxMembers = 200
	}

	group, err := h.groupService.Create(req.Name, req.Avatar, userID, maxMembers)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	// 创建者自动成为群主
	h.groupService.AddMember(group.ID, userID, RoleOwner)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "创建成功",
		"data":    group,
	})
}

// GetList 获取群组列表
func (h *GroupHandler) GetList(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	groups, err := h.groupService.GetByOwner(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data":  groups,
	})
}

// GetMembers 获取群成员
func (h *GroupHandler) GetMembers(c *gin.Context) {
	groupIDStr := c.Param("groupId")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的群组ID"})
		return
	}

	members, err := h.groupService.GetMembers(groupID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data":  members,
	})
}

// Join 加入群组
func (h *GroupHandler) Join(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	groupIDStr := c.Param("groupId")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的群组ID"})
		return
	}

	// 检查是否已经是成员
	isMember, _ := h.groupService.IsMember(groupID, userID)
	if isMember {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": "已经是群成员"})
		return
	}

	err = h.groupService.AddMember(groupID, userID, RoleMember)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "加入成功",
	})
}

// Leave 离开群组
func (h *GroupHandler) Leave(c *gin.Context) {
	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	groupIDStr := c.Param("groupId")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的群组ID"})
		return
	}

	err = h.groupService.RemoveMember(groupID, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "已离开群组",
	})
}
