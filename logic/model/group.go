package model

import (
	"time"

	"gochat/pkg/mysql"
)

// Group 群组模型
type Group struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name       string    `gorm:"column:name;size:128;not null" json:"name"`
	Avatar     string    `gorm:"column:avatar;size:255" json:"avatar"`
	OwnerID    int64     `gorm:"column:owner_id;not null;index:idx_owner" json:"owner_id"`
	MaxMembers int       `gorm:"column:max_members;default:500" json:"max_members"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (Group) TableName() string {
	return "group"
}

// CreateGroup 创建群组
func CreateGroup(group *Group) error {
	return mysql.Client.Create(group).Error
}

// GetGroupByID 根据ID获取群组
func GetGroupByID(id int64) (*Group, error) {
	var group Group
	err := mysql.Client.Where("id = ?", id).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetGroupsByOwner 获取用户拥有的群组
func GetGroupsByOwner(ownerID int64) ([]*Group, error) {
	var groups []*Group
	err := mysql.Client.Where("owner_id = ?", ownerID).Find(&groups).Error
	return groups, err
}

// UpdateGroup 更新群组信息
func UpdateGroup(id int64, name, avatar string) error {
	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	return mysql.Client.Model(&Group{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteGroup 删除群组
func DeleteGroup(id int64) error {
	return mysql.Client.Delete(&Group{}, id).Error
}

// TransferGroup 转让群组
func TransferGroup(groupID, newOwnerID int64) error {
	return mysql.Client.Model(&Group{}).Where("id = ?", groupID).Update("owner_id", newOwnerID).Error
}
