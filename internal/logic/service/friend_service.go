package service

import (
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	apierrors "gochat/internal/pkg/errors"
	"gochat/internal/pkg/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"unicode/utf8"
)

// FriendService 好友服务
type FriendService struct {
	friendRepo registry.FriendRepository
}

// NewFriendService 创建好友服务实例
func NewFriendService(friendRepo registry.FriendRepository) *FriendService {
	return &FriendService{
		friendRepo: friendRepo,
	}
}

// Add 添加好友
func (s *FriendService) Add(userID, friendID int64, remark string) error {
	if utf8.RuneCountInString(remark) > 64 {
		return apierrors.New(apierrors.CodeBadRequest, "备注不能超过64个字符")
	}
	return mysql.Client.Transaction(func(tx *gorm.DB) error {
		if _, err := lockPrivatePair(tx, userID, friendID); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Friend{UserID: userID, FriendID: friendID, Remark: remark}).Error
	})
}

// GetList 获取好友列表
func (s *FriendService) GetList(userID int64) ([]*model.Friend, error) {
	friends, err := s.friendRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	var incoming []model.Friend
	if err = mysql.Client.Where("friend_id=?", userID).Find(&incoming).Error; err != nil {
		return nil, err
	}
	peers := map[int64]bool{}
	for _, f := range incoming {
		peers[f.UserID] = true
	}
	for _, f := range friends {
		f.AddedByPeer = peers[f.FriendID]
	}
	return friends, nil
}

// Remove 删除好友
func (s *FriendService) Remove(userID, friendID int64) error {
	return mysql.Client.Transaction(func(tx *gorm.DB) error {
		if _, err := lockPrivatePair(tx, userID, friendID); err != nil {
			return err
		}
		return tx.Where("user_id=? AND friend_id=?", userID, friendID).Delete(&model.Friend{}).Error
	})
}

type FriendNotice struct {
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar"`
	Unread    bool      `json:"unread"`
	AddedByMe bool      `json:"added_by_me"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *FriendService) Incoming(userID int64) ([]FriendNotice, error) {
	items := make([]FriendNotice, 0)
	err := mysql.Client.Raw("SELECT f.user_id,COALESCE(NULLIF(u.nickname,''),u.username) name,u.avatar,f.recipient_seen_at IS NULL unread,f.created_at,EXISTS(SELECT 1 FROM friend r WHERE r.user_id=? AND r.friend_id=f.user_id) added_by_me FROM friend f JOIN user u ON u.id=f.user_id WHERE f.friend_id=? ORDER BY f.created_at DESC,f.id DESC", userID, userID).Scan(&items).Error
	return items, err
}
func (s *FriendService) ReadNotice(userID, fromID int64) error {
	return mysql.Client.Model(&model.Friend{}).Where("friend_id=? AND user_id=? AND recipient_seen_at IS NULL", userID, fromID).Update("recipient_seen_at", time.Now()).Error
}

// UpdateRemark 更新好友备注
func (s *FriendService) UpdateRemark(userID, friendID int64, remark string) error {
	return s.friendRepo.UpdateRemark(userID, friendID, remark)
}

// IsFriend 检查是否是好友
func (s *FriendService) IsFriend(userID, friendID int64) (bool, error) {
	return s.friendRepo.IsFriend(userID, friendID)
}
