package registry

import (
	"gochat/internal/logic/model"
	"gochat/internal/pkg/mysql"
)

// GroupRepository 群组仓储接口
type GroupRepository interface {
	Create(group *model.Group) error
	GetByID(id int64) (*model.Group, error)
	GetByOwnerID(ownerID int64) ([]*model.Group, error)
	GetByUserID(userID int64) ([]*model.Group, error)
	Update(id int64, name, avatar string) error
	Delete(id int64) error
	TransferOwnership(groupID, newOwnerID int64) error
}

// groupRepository 群组仓储默认实现
type groupRepository struct{}

// NewGroupRepository 创建群组仓储实例
func NewGroupRepository() GroupRepository {
	return &groupRepository{}
}

// Create 创建群组
func (r *groupRepository) Create(group *model.Group) error {
	return mysql.Client.Create(group).Error
}

// GetByID 根据ID获取群组
func (r *groupRepository) GetByID(id int64) (*model.Group, error) {
	var group model.Group
	err := mysql.Client.Where("id = ?", id).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetByOwnerID 获取用户拥有的群组
func (r *groupRepository) GetByOwnerID(ownerID int64) ([]*model.Group, error) {
	var groups []*model.Group
	err := mysql.Client.Where("owner_id = ?", ownerID).Find(&groups).Error
	return groups, err
}

// GetByUserID 获取用户加入的群组（通过群成员表关联）
func (r *groupRepository) GetByUserID(userID int64) ([]*model.Group, error) {
	var groups []*model.Group
	err := mysql.Client.
		Joins("JOIN group_member ON group_member.group_id = `group`.id").
		Where("group_member.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}

// Update 更新群组信息
func (r *groupRepository) Update(id int64, name, avatar string) error {
	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	return mysql.Client.Model(&model.Group{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除群组
func (r *groupRepository) Delete(id int64) error {
	return mysql.Client.Delete(&model.Group{}, id).Error
}

// TransferOwnership 转让群组所有权
func (r *groupRepository) TransferOwnership(groupID, newOwnerID int64) error {
	return mysql.Client.Model(&model.Group{}).Where("id = ?", groupID).Update("owner_id", newOwnerID).Error
}
