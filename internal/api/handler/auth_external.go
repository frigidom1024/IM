package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/errors"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/util"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ExternalAuthHandler 外部认证处理器
type ExternalAuthHandler struct {
	userRepo   registry.UserRepository
	apiURL     string
	httpClient *http.Client
}

// NewExternalAuthHandler 创建外部认证处理器
func NewExternalAuthHandler(userRepo registry.UserRepository, apiURL string) *ExternalAuthHandler {
	return &ExternalAuthHandler{
		userRepo:   userRepo,
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ExternalAuth 通过外部API认证
func (h *ExternalAuthHandler) ExternalAuth(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		errors.RespondError(c, errors.New(errors.CodeUnauthorized, "缺少认证token"))
		return
	}

	extUser, err := h.fetchExternalUser(token)
	if err != nil {
		logger.Warn("获取外部用户信息失败", zap.Error(err))
		errors.RespondError(c, errors.New(errors.CodeUnauthorized, "认证失败"))
		return
	}

	// 用 ext_ 前缀避免与注册用户冲突，不改表结构
	extUsername := "ext_" + extUser.ID

	user, err := h.userRepo.GetByUsername(extUsername)
	if err != nil {
		user = &model.User{
			Username: extUsername,
			Nickname: extUser.Name,
			Avatar:   extUser.Avatar,
		}
		if err := h.userRepo.Create(user); err != nil {
			logger.Error("创建用户失败", zap.Error(err))
			errors.RespondError(c, errors.New(errors.CodeServerError, "创建用户失败"))
			return
		}
	} else {
		changed := false
		if extUser.Name != "" && extUser.Name != user.Nickname {
			user.Nickname = extUser.Name
			changed = true
		}
		if extUser.Avatar != "" && extUser.Avatar != user.Avatar {
			user.Avatar = extUser.Avatar
			changed = true
		}
		if changed {
			h.userRepo.UpdateProfile(user.ID, user.Nickname, user.Avatar)
		}
	}

	jwtToken, err := util.GenerateToken(int(user.ID), user.Username, 48)
	if err != nil {
		errors.RespondError(c, errors.New(errors.CodeServerError, "生成token失败"))
		return
	}

	errors.RespondOK(c, gin.H{
		"token": jwtToken,
		"user": gin.H{
			"id":       user.ID,
			"name":     user.Nickname,
			"avatar":   user.Avatar,
			"username": user.Username,
		},
	})
}

// ExternalUser 外部系统用户信息
type ExternalUser struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

func (h *ExternalAuthHandler) fetchExternalUser(token string) (*ExternalUser, error) {
	req, err := http.NewRequest("GET", h.apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求外部API失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("外部API返回 %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var extUser ExternalUser
	if err := json.Unmarshal(body, &extUser); err != nil {
		return nil, fmt.Errorf("解析外部用户信息失败: %w", err)
	}

	if extUser.ID == "" {
		return nil, fmt.Errorf("外部API未返回用户ID")
	}

	return &extUser, nil
}
