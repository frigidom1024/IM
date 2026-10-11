package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"gochat/internal/logic/model"
	apierrors "gochat/internal/pkg/errors"
	"gochat/internal/pkg/mysql"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

// ParseConversationScope accepts the public stable conversation identifier.
func ParseConversationScope(scope string) (kind string, id int64, err error) {
	parts := strings.Split(scope, ":")
	if len(parts) != 2 || (parts[0] != "private" && parts[0] != "group") {
		return "", 0, fmt.Errorf("无效的会话ID")
	}
	id, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[1] {
		return "", 0, fmt.Errorf("无效的会话ID")
	}
	return parts[0], id, nil
}

func UpdateConversation(c *gin.Context) {
	kind, id, err := ParseConversationScope(c.Param("scope"))
	if err != nil {
		apierrors.RespondBadRequest(c, err.Error())
		return
	}
	var patch struct {
		Pinned *bool   `json:"pinned"`
		Muted  *bool   `json:"muted"`
		Draft  *string `json:"draft"`
	}
	if c.ShouldBindJSON(&patch) != nil || (patch.Pinned == nil && patch.Muted == nil && patch.Draft == nil) || (patch.Draft != nil && len(*patch.Draft) > 4096) {
		apierrors.RespondBadRequest(c, "无效的会话偏好")
		return
	}
	userID := c.GetInt64("userID")
	db := mysql.Client.WithContext(c.Request.Context())
	var count int64
	if kind == "group" {
		err = db.Model(&model.GroupMember{}).Where("group_id=? AND user_id=?", id, userID).Count(&count).Error
	} else {
		err = db.Model(&model.User{}).Where("id=?", id).Count(&count).Error
	}
	if err != nil {
		apierrors.RespondError(c, apierrors.New(apierrors.CodeServerError, "读取会话失败"))
		return
	}
	if count == 0 {
		c.JSON(403, gin.H{"code": 40300, "message": "无权访问会话"})
		return
	}
	updates := map[string]any{"updated_at": time.Now()}
	if patch.Pinned != nil {
		updates["pinned"] = *patch.Pinned
	}
	if patch.Muted != nil {
		updates["muted"] = *patch.Muted
	}
	if patch.Draft != nil {
		updates["draft"] = *patch.Draft
	}
	row := model.ConversationPreference{UserID: userID, Scope: c.Param("scope")}
	if patch.Pinned != nil {
		row.Pinned = *patch.Pinned
	}
	if patch.Muted != nil {
		row.Muted = *patch.Muted
	}
	if patch.Draft != nil {
		row.Draft = *patch.Draft
	}
	err = db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "scope"}}, DoUpdates: clause.Assignments(updates)}).Create(&row).Error
	if err != nil {
		apierrors.RespondError(c, apierrors.New(apierrors.CodeServerError, "保存会话失败"))
		return
	}
	apierrors.RespondOK(c, nil)
}

func ListConversations(c *gin.Context) {
	limit, offset, err := parseMessagePage(c)
	if err != nil {
		apierrors.RespondBadRequest(c, err.Error())
		return
	}
	userID := c.GetInt64("userID")
	// Union includes outgoing-only conversations, friends, joined groups and drafts.
	// The group membership join also removes scopes after the viewer leaves a group.
	query := "WITH scopes AS (" +
		"SELECT CONCAT('private:',IF(from_id=?,to_id,from_id)) scope FROM message WHERE group_id=0 AND (from_id=? OR to_id=?) " +
		"UNION SELECT CONCAT('private:',friend_id) FROM friend WHERE user_id=? " +
		"UNION SELECT CONCAT('group:',group_id) FROM group_member WHERE user_id=? " +
		"UNION SELECT scope FROM conversation_preference WHERE user_id=?), " +
		"ranked AS (SELECT m.*,IF(group_id>0,CONCAT('group:',group_id),CONCAT('private:',IF(from_id=?,to_id,from_id))) scope," +
		"ROW_NUMBER() OVER (PARTITION BY IF(group_id>0,CONCAT('group:',group_id),CONCAT('private:',IF(from_id=?,to_id,from_id))) ORDER BY created_at DESC,id DESC) rn " +
		"FROM message m WHERE (group_id=0 AND (from_id=? OR to_id=?)) OR group_id IN (SELECT group_id FROM group_member WHERE user_id=?)) " +
		"SELECT s.scope,COALESCE(NULLIF(u.nickname,''),u.username,g.name,'') name,COALESCE(u.avatar,g.avatar,'') avatar," +
		"COALESCE(uc.unread_count,0) unread_count,COALESCE(p.pinned,0) pinned,COALESCE(p.muted,0) muted,COALESCE(p.draft,'') draft," +
		"r.id message_id,r.from_id,r.to_id,r.group_id,r.msg_type,r.content,r.is_revoked,r.created_at " +
		"FROM scopes s LEFT JOIN conversation_preference p ON p.user_id=? AND p.scope=s.scope " +
		"LEFT JOIN ranked r ON r.scope=s.scope AND r.rn=1 " +
		"LEFT JOIN user u ON s.scope=CONCAT('private:',u.id) " +
		"LEFT JOIN `group` g ON s.scope=CONCAT('group:',g.id) " +
		"LEFT JOIN group_member gm ON gm.group_id=g.id AND gm.user_id=? " +
		"LEFT JOIN unread_count uc ON uc.user_id=? AND s.scope=IF(uc.group_id>0,CONCAT('group:',uc.group_id),CONCAT('private:',uc.from_id)) " +
		"WHERE (u.id IS NOT NULL OR gm.user_id IS NOT NULL) ORDER BY pinned DESC,r.created_at DESC,r.id DESC,s.scope LIMIT ? OFFSET ?"
	var rows []struct {
		Scope, Name, Avatar, Draft string
		UnreadCount                int64
		Pinned, Muted              bool
		MessageID                  *int64
		FromID, ToID, GroupID      int64
		MsgType, Content           string
		IsRevoked                  bool
		CreatedAt                  *time.Time
	}
	err = mysql.Client.WithContext(c.Request.Context()).Raw(query, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, limit, offset).Scan(&rows).Error
	if err != nil {
		apierrors.RespondError(c, apierrors.New(apierrors.CodeServerError, "读取会话失败"))
		return
	}
	result := make([]model.Conversation, 0, len(rows))
	for _, r := range rows {
		item := model.Conversation{ID: r.Scope, Name: r.Name, Avatar: r.Avatar, UnreadCount: r.UnreadCount, Pinned: r.Pinned, Muted: r.Muted, Draft: r.Draft, LastMessageAt: r.CreatedAt}
		if r.MessageID != nil && r.CreatedAt != nil {
			item.LastMessage = &model.Message{ID: *r.MessageID, FromID: r.FromID, ToID: r.ToID, GroupID: r.GroupID, MsgType: r.MsgType, Content: r.Content, IsRevoked: r.IsRevoked, CreatedAt: *r.CreatedAt}
			if r.IsRevoked {
				item.LastMessage.Content = ""
			}
		}
		result = append(result, item)
	}
	apierrors.RespondOK(c, result)
}
