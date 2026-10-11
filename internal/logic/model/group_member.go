package model

import (
	"time"
)

// GroupMemberRole 群成员角色
const (
	RoleMember int8 = 0 // 普通成员
	RoleAdmin  int8 = 1 // 管理员
	RoleOwner  int8 = 2 // 群主
)

// GroupMember 群成员模型
type GroupMember struct {
	Nickname          string     `gorm:"column:nickname;size:128" json:"nickname"`
	MutedUntil        *time.Time `gorm:"column:muted_until" json:"muted_until,omitempty"`
	LastReadMessageID int64      `gorm:"column:last_read_message_id;default:0" json:"last_read_message_id,string"`
	ReadAt            *time.Time `gorm:"column:read_at" json:"read_at,omitempty"`
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	GroupID           int64      `gorm:"column:group_id;not null;index:idx_group,priority:1;uniqueIndex:uk_group_user,priority:1" json:"group_id"`
	UserID            int64      `gorm:"column:user_id;not null;index:idx_user;uniqueIndex:uk_group_user,priority:2" json:"user_id"`
	Role              int8       `gorm:"column:role;default:0" json:"role"` // 0普通 1管理员 2群主
	JoinedAt          time.Time  `gorm:"column:joined_at;autoCreateTime" json:"joined_at"`
}

// TableName 指定表名
func (GroupMember) TableName() string {
	return "group_member"
}
