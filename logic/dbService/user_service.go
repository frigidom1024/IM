package dbService

import (
	"errors"
	"fmt"
	"gochat/logic/model"
	"gochat/pkg/util"
)

var (
	ErrUserExists        = errors.New("用户名已存在")
	ErrUserNotExists     = errors.New("用户不存在")
	ErrPasswordIncorrect = errors.New("密码错误")
)

type RegisterReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserRes struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Status   int8   `json:"status"`
}

func Register(req *RegisterReq) (*UserRes, error) {
	_, err := model.GetUserByUsername(req.Username)
	if err == nil {
		return nil, ErrUserExists
	}

	pwdhash, err := util.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	user := &model.User{
		Username: req.Username,
		Password: pwdhash,
		Nickname: req.Nickname,
		Status:   0,
	}

	if err := model.CreateUser(user); err != nil {
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

func Login(req *LoginReq) (*UserRes, string, error) {
	user, err := model.GetUserByUsername(req.Username)
	if err != nil {
		return nil, "", ErrUserNotExists
	}

	if !util.CheckPassword(user.Password, req.Password) {
		return nil, "", ErrPasswordIncorrect
	}

	// 更新在线状态
	if err := model.UpdateUserStatus(user.ID, 1); err != nil {
		return nil, "", fmt.Errorf("更新状态失败: %w", err)
	}

	token, err := util.GenerateToken(int(user.ID), user.Username, 48)
	if err != nil {
		return nil, "", fmt.Errorf("生成Token失败: %w", err)
	}

	// 重新获取用户数据以获得最新状态
	user, err = model.GetUserByID(user.ID)
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

func GetUserInfo(id int64) (*UserRes, error) {
	user, err := model.GetUserByID(id)
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

func UpdateProfile(id int64, nickname string, avatar string) error {
	return model.UpdateUserProfile(id, nickname, avatar)
}

func UpdateStatus(id int64, status int8) error {
	return model.UpdateUserStatus(id, status)
}
