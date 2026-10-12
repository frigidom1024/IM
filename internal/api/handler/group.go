package handler

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	RoleOwner  = model.RoleOwner
	RoleAdmin  = model.RoleAdmin
	RoleMember = model.RoleMember
)

type GroupHandler struct {
	groupService *service.GroupService
	admin        service.GroupAdminService
	notify       Notify
}

func NewGroupHandler(g registry.GroupRepository, m registry.GroupMemberRepository, notifications ...Notify) *GroupHandler {
	h := &GroupHandler{groupService: service.NewGroupService(g, m)}
	if len(notifications) > 0 {
		h.notify = notifications[0]
	}
	return h
}
func groupResult(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(200, gin.H{"code": 0, "data": data})
		return
	}
	status := 500
	message := "group operation failed"
	switch {
	case errors.Is(err, service.ErrGroupForbidden):
		status = 403
		message = err.Error()
	case errors.Is(err, service.ErrGroupInvalid):
		status = 400
		message = err.Error()
	case errors.Is(err, service.ErrGroupFull):
		status = 409
		message = err.Error()
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = 404
		message = "group or member not found"
	}
	c.JSON(status, gin.H{"code": status, "message": message})
}
func groupScope(c *gin.Context) (int64, int64, bool) {
	user := c.GetInt64("userID")
	if user <= 0 {
		c.JSON(401, gin.H{"code": 401, "message": "authentication required"})
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("groupId"), 10, 64)
	if err != nil || id <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return 0, 0, false
	}
	return id, user, true
}
func groupPage(c *gin.Context) (int, int, bool) {
	limit, offset, err := parseMessagePage(c)
	if err != nil || offset > 1000000 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return 0, 0, false
	}
	return limit, int(offset), true
}
func (h *GroupHandler) Create(c *gin.Context) {
	user := c.GetInt64("userID")
	if user <= 0 {
		c.JSON(401, gin.H{"code": 401, "message": "authentication required"})
		return
	}
	var req struct {
		Name       string `json:"name"`
		Avatar     string `json:"avatar"`
		MaxMembers int    `json:"max_members"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || req.MaxMembers < 0 || req.MaxMembers > 10000 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	if req.MaxMembers == 0 {
		req.MaxMembers = 200
	}
	group, err := h.admin.Create(req.Name, req.Avatar, user, req.MaxMembers)
	groupResult(c, group, err)
	if err == nil {
		h.notifyGroup(group.ID, user)
	}
}
func (h *GroupHandler) GetList(c *gin.Context) {
	groups, err := h.admin.ListGroups(c.GetInt64("userID"))
	groupResult(c, groups, err)
}
func (h *GroupHandler) GetMembers(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	limit, offset, ok := groupPage(c)
	if !ok {
		return
	}
	members, err := h.admin.Members(id, user, limit, offset)
	groupResult(c, members, err)
}
func (h *GroupHandler) Join(c *gin.Context) {
	id, user, ok := groupScope(c)
	if ok {
		application, err := h.admin.Apply(id, user)
		groupResult(c, application, err)
		if err == nil {
			h.notifyGroup(id, user)
		}
	}
}

func (h *GroupHandler) MyApplications(c *gin.Context) {
	limit, offset, ok := groupPage(c)
	if !ok {
		return
	}
	items, err := h.admin.MyApplications(c.GetInt64("userID"), limit, offset)
	groupResult(c, items, err)
}
func (h *GroupHandler) Applications(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	limit, offset, ok := groupPage(c)
	if !ok {
		return
	}
	items, err := h.admin.Applications(id, user, limit, offset)
	groupResult(c, items, err)
}
func (h *GroupHandler) Review(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	requestID, err := strconv.ParseInt(c.Param("applicationId"), 10, 64)
	if err != nil || requestID <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if c.ShouldBindJSON(&req) != nil {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	item, err := h.admin.Review(id, user, requestID, req.Decision, strings.TrimSpace(req.Reason))
	groupResult(c, item, err)
	if err == nil {
		h.notifyGroup(id, item.UserID)
	}
}
func (h *GroupHandler) Leave(c *gin.Context) {
	id, user, ok := groupScope(c)
	if ok {
		err := h.admin.Remove(id, user, user)
		groupResult(c, nil, err)
		if err == nil {
			h.notifyGroup(id, user)
		}
	}
}
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	target, err := strconv.ParseInt(c.Param("userId"), 10, 64)
	if err != nil || target <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	err = h.admin.Remove(id, user, target)
	groupResult(c, nil, err)
	if err == nil {
		h.notifyGroup(id, target)
	}
}
func (h *GroupHandler) Search(c *gin.Context) {
	limit, offset, ok := groupPage(c)
	if !ok {
		return
	}
	groups, err := h.admin.Search(strings.TrimSpace(c.Query("q")), limit, offset)
	groupResult(c, groups, err)
}
func (h *GroupHandler) Invite(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if c.ShouldBindJSON(&req) != nil || req.UserID <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	err := h.admin.Invite(id, user, req.UserID)
	groupResult(c, nil, err)
	if err == nil {
		h.notifyGroup(id, req.UserID)
	}
}
func (h *GroupHandler) Transfer(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if c.ShouldBindJSON(&req) != nil || req.UserID <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	err := h.admin.Transfer(id, user, req.UserID)
	groupResult(c, nil, err)
	if err == nil {
		h.notifyGroup(id, user, req.UserID)
	}
}
func (h *GroupHandler) Announcement(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	var req struct {
		Announcement *string `json:"announcement"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Announcement == nil {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	groupResult(c, nil, h.admin.Announce(id, user, *req.Announcement))
}
func (h *GroupHandler) UpdateMember(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	target, err := strconv.ParseInt(c.Param("userId"), 10, 64)
	if err != nil || target <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	var req struct {
		Nickname   *string    `json:"nickname"`
		MutedUntil *time.Time `json:"muted_until"`
		Role       *int8      `json:"role"`
		Unmute     bool       `json:"unmute"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Nickname == nil && req.MutedUntil == nil && req.Role == nil && !req.Unmute) || (req.Unmute && req.MutedUntil != nil) {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	err = h.admin.UpdateMember(id, user, target, req.Nickname, req.MutedUntil, req.Unmute || req.MutedUntil != nil, req.Role)
	groupResult(c, nil, err)
	if err == nil {
		h.notifyGroup(id, target)
	}
}
func (h *GroupHandler) Dissolve(c *gin.Context) {
	id, user, ok := groupScope(c)
	if ok {
		var recipients []int64
		members, _ := h.groupService.GetMembers(id)
		for _, member := range members {
			recipients = append(recipients, member.UserID)
		}
		requests, _ := h.admin.Applications(id, user, 1000000, 0)
		for _, request := range requests {
			if request.Status == "pending" {
				recipients = append(recipients, request.UserID)
			}
		}
		err := h.admin.Dissolve(id, user)
		groupResult(c, nil, err)
		if err == nil {
			h.notify.send("group_changed", gin.H{"group_id": id}, recipients...)
		}
	}
}
func (h *GroupHandler) Receipts(c *gin.Context) {
	id, user, ok := groupScope(c)
	if !ok {
		return
	}
	messageID, err := strconv.ParseInt(c.Param("messageId"), 10, 64)
	if err != nil || messageID <= 0 {
		groupResult(c, nil, service.ErrGroupInvalid)
		return
	}
	limit, offset, ok := groupPage(c)
	if !ok {
		return
	}
	readers, err := h.admin.Receipts(id, user, messageID, limit, offset)
	groupResult(c, readers, err)
}
