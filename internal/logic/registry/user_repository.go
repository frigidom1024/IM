package registry

import (
	"gochat/internal/logic/model"
	"gochat/internal/pkg/mysql"
)

// UserRepository 用户仓储接口
type UserRepository interface {
	Create(user *model.User) error
	GetByID(id int64) (*model.User, error)
	GetByUsername(username string) (*model.User, error)
	UpdateStatus(id int64, status int8) error
	UpdateProfile(id int64, nickname, avatar string) error
}

// userRepository 用户仓储默认实现
type userRepository struct{}

// NewUserRepository 创建用户仓储实例
func NewUserRepository() UserRepository {
	return &userRepository{}
}

// Create 创建用户
func (r *userRepository) Create(user *model.User) error {
	return mysql.Client.Create(user).Error
}

// GetByID 根据ID获取用户
func (r *userRepository) GetByID(id int64) (*model.User, error) {
	var user model.User
	err := mysql.Client.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUsername 根据用户名获取用户
func (r *userRepository) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := mysql.Client.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateStatus 更新用户状态
func (r *userRepository) UpdateStatus(id int64, status int8) error {
	return mysql.Client.Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

// UpdateProfile 更新用户资料
func (r *userRepository) UpdateProfile(id int64, nickname, avatar string) error {
	updates := map[string]interface{}{}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	return mysql.Client.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}
