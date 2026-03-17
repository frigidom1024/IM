package registry

import (
	"gochat/logic/model"
	"gochat/pkg/mysql"
)

// GroupMemberRepository 群成员仓储接口
type GroupMemberRepository interface {
	Add(member *model.GroupMember) error
	GetByGroupID(groupID int64) ([]*model.GroupMember, error)
	Get(groupID, userID int64) (*model.GroupMember, error)
	Remove(groupID, userID int64) error
	UpdateRole(groupID, userID int64, role int8) error
	IsMember(groupID, userID int64) (bool, error)
	GetCount(groupID int64) (int64, error)
	GetByUserID(userID int64) ([]*model.GroupMember, error)
}

// groupMemberRepository 群成员仓储默认实现
type groupMemberRepository struct{}

// NewGroupMemberRepository 创建群成员仓储实例
func NewGroupMemberRepository() GroupMemberRepository {
	return &groupMemberRepository{}
}

// Add 添加群成员
func (r *groupMemberRepository) Add(member *model.GroupMember) error {
	return mysql.Client.Create(member).Error
}

// GetByGroupID 获取群成员列表
func (r *groupMemberRepository) GetByGroupID(groupID int64) ([]*model.GroupMember, error) {
	var members []*model.GroupMember
	err := mysql.Client.Where("group_id = ?", groupID).Find(&members).Error
	return members, err
}

// Get 获取群成员
func (r *groupMemberRepository) Get(groupID, userID int64) (*model.GroupMember, error) {
	var member model.GroupMember
	err := mysql.Client.Where("group_id = ? AND user_id = ?", groupID, userID).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// Remove 移除群成员
func (r *groupMemberRepository) Remove(groupID, userID int64) error {
	return mysql.Client.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&model.GroupMember{}).Error
}

// UpdateRole 更新成员角色
func (r *groupMemberRepository) UpdateRole(groupID, userID int64, role int8) error {
	return mysql.Client.Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("role", role).Error
}

// IsMember 检查是否是群成员
func (r *groupMemberRepository) IsMember(groupID, userID int64) (bool, error) {
	var count int64
	err := mysql.Client.Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Count(&count).Error
	return count > 0, err
}

// GetCount 获取群成员数量
func (r *groupMemberRepository) GetCount(groupID int64) (int64, error) {
	var count int64
	err := mysql.Client.Model(&model.GroupMember{}).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
}

// GetByUserID 获取用户加入的群组
func (r *groupMemberRepository) GetByUserID(userID int64) ([]*model.GroupMember, error) {
	var members []*model.GroupMember
	err := mysql.Client.Where("user_id = ?", userID).Find(&members).Error
	return members, err
}
