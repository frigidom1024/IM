package model

import (
	"time"

	"gochat/pkg/mysql"
)

// GroupMemberRole 群成员角色
const (
	RoleMember   int8 = 0 // 普通成员
	RoleAdmin    int8 = 1 // 管理员
	RoleOwner    int8 = 2 // 群主
)

// GroupMember 群成员模型
type GroupMember struct {
	ID       int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	GroupID  int64     `gorm:"column:group_id;not null;index:idx_group,priority:1;uniqueIndex:uk_group_user,priority:1" json:"group_id"`
	UserID   int64     `gorm:"column:user_id;not null;index:idx_user;uniqueIndex:uk_group_user,priority:2" json:"user_id"`
	Role     int8      `gorm:"column:role;default:0" json:"role"` // 0普通 1管理员 2群主
	JoinedAt time.Time `gorm:"column:joined_at;autoCreateTime" json:"joined_at"`
}

// TableName 指定表名
func (GroupMember) TableName() string {
	return "group_member"
}

// AddGroupMember 添加群成员
func AddGroupMember(member *GroupMember) error {
	return mysql.Client.Create(member).Error
}

// GetGroupMembers 获取群成员列表
func GetGroupMembers(groupID int64) ([]*GroupMember, error) {
	var members []*GroupMember
	err := mysql.Client.Where("group_id = ?", groupID).Find(&members).Error
	return members, err
}

// GetGroupMember 获取群成员
func GetGroupMember(groupID, userID int64) (*GroupMember, error) {
	var member GroupMember
	err := mysql.Client.Where("group_id = ? AND user_id = ?", groupID, userID).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// RemoveGroupMember 移除群成员
func RemoveGroupMember(groupID, userID int64) error {
	return mysql.Client.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&GroupMember{}).Error
}

// UpdateMemberRole 更新成员角色
func UpdateMemberRole(groupID, userID int64, role int8) error {
	return mysql.Client.Model(&GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("role", role).Error
}

// IsGroupMember 检查是否是群成员
func IsGroupMember(groupID, userID int64) (bool, error) {
	var count int64
	err := mysql.Client.Model(&GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Count(&count).Error
	return count > 0, err
}

// GetGroupMemberCount 获取群成员数量
func GetGroupMemberCount(groupID int64) (int64, error) {
	var count int64
	err := mysql.Client.Model(&GroupMember{}).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
}

// GetUserGroups 获取用户加入的群组
func GetUserGroups(userID int64) ([]*GroupMember, error) {
	var members []*GroupMember
	err := mysql.Client.Where("user_id = ?", userID).Find(&members).Error
	return members, err
}
