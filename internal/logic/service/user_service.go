package service

import (
	"errors"
	"fmt"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/util"
)

var (
	ErrUserExists        = errors.New("用户名已存在")
	ErrUserNotExists     = errors.New("用户不存在")
	ErrPasswordIncorrect = errors.New("密码错误")
)

// UserService 用户服务
type UserService struct {
	userRepo registry.UserRepository
}

// NewUserService 创建用户服务实例
func NewUserService(userRepo registry.UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
	}
}

// RegisterReq 注册请求
type RegisterReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// LoginReq 登录请求
type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UserRes 用户响应
type UserRes struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Status   int8   `json:"status"`
}

// Register 用户注册
func (s *UserService) Register(req *RegisterReq) (*UserRes, error) {
	// 检查用户是否存在
	_, err := s.userRepo.GetByUsername(req.Username)
	if err == nil {
		return nil, ErrUserExists
	}

	// 加密密码
	pwdHash, err := util.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	// 创建用户
	user := &model.User{
		Username: req.Username,
		Password: pwdHash,
		Nickname: req.Nickname,
		Status:   0,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	return &UserRes{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		Status:   user.Status,
	}, nil
}

// Login 用户登录
func (s *UserService) Login(req *LoginReq) (*UserRes, string, error) {
	// 获取用户
	user, err := s.userRepo.GetByUsername(req.Username)
	if err != nil {
		return nil, "", ErrUserNotExists
	}

	// 验证密码
	if !util.CheckPassword(user.Password, req.Password) {
		return nil, "", ErrPasswordIncorrect
	}

	// 更新在线状态
	if err := s.userRepo.UpdateStatus(user.ID, 1); err != nil {
		return nil, "", fmt.Errorf("更新状态失败: %w", err)
	}

	// 生成Token
	token, err := util.GenerateToken(int(user.ID), user.Username, 48)
	if err != nil {
		return nil, "", fmt.Errorf("生成Token失败: %w", err)
	}

	// 重新获取用户数据以获得最新状态
	user, err = s.userRepo.GetByID(user.ID)
	if err != nil {
		return nil, "", fmt.Errorf("获取用户信息失败: %w", err)
	}

	return &UserRes{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		Status:   user.Status,
	}, token, nil
}

// GetUserInfo 获取用户信息
func (s *UserService) GetUserInfo(id int64) (*UserRes, error) {
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return &UserRes{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		Status:   user.Status,
	}, nil
}

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(id int64, nickname string, avatar string) error {
	return s.userRepo.UpdateProfile(id, nickname, avatar)
}

// UpdateStatus 更新用户状态
func (s *UserService) UpdateStatus(id int64, status int8) error {
	return s.userRepo.UpdateStatus(id, status)
}
