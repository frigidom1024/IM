package service

import (
	"errors"
	"gochat/internal/logic/model"
	apierrors "gochat/internal/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SendUnavailable  = "unavailable"
	SendTextOnce     = "text_once"
	SendUnrestricted = "unrestricted"
)

type PrivatePolicy struct {
	SendPolicy     string `json:"send_policy"`
	IntroRemaining int    `json:"intro_remaining"`
	AddedByMe      bool   `json:"added_by_me"`
	AddedByPeer    bool   `json:"added_by_peer"`
}

func lockPrivatePair(tx *gorm.DB, a, b int64) (*model.PrivatePair, error) {
	if a <= 0 || b <= 0 || a == b {
		return nil, apierrors.New(apierrors.CodeFriendSelf, "不能添加或私信自己")
	}
	var users int64
	if err := tx.Model(&model.User{}).Where("id IN ?", []int64{a, b}).Count(&users).Error; err != nil {
		return nil, err
	}
	if users != 2 {
		return nil, apierrors.New(apierrors.CodeUserNotFound, "用户不存在")
	}
	low, high := a, b
	if low > high {
		low, high = high, low
	}
	pair := model.PrivatePair{LowID: low, HighID: high}
	// An UPDATE on conflict acquires the exclusive lock immediately, avoiding
	// concurrent INSERT IGNORE readers upgrading shared locks on the same row.
	if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"low_id"})}).Create(&pair).Error; err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pair, "low_id=? AND high_id=?", low, high).Error; err != nil {
		return nil, err
	}
	return &pair, nil
}

func ReadPrivatePolicy(db *gorm.DB, from, to int64) (PrivatePolicy, error) {
	p := PrivatePolicy{SendPolicy: SendUnavailable}
	var edges []model.Friend
	if err := db.Session(&gorm.Session{}).Where("(user_id=? AND friend_id=?) OR (user_id=? AND friend_id=?)", from, to, to, from).Find(&edges).Error; err != nil {
		return p, err
	}
	for _, edge := range edges {
		if edge.UserID == from {
			p.AddedByMe = true
		} else {
			p.AddedByPeer = true
		}
	}
	if p.AddedByMe && p.AddedByPeer {
		p.SendPolicy = SendUnrestricted
		return p, nil
	}
	if !p.AddedByMe {
		return p, nil
	}
	low, high := from, to
	if low > high {
		low, high = high, low
	}
	var pair model.PrivatePair
	err := db.Session(&gorm.Session{}).First(&pair, "low_id=? AND high_id=?", low, high).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return p, err
	}
	used := pair.LowIntroUsed
	if from == high {
		used = pair.HighIntroUsed
	}
	if !used {
		p.SendPolicy = SendTextOnce
		p.IntroRemaining = 1
	}
	return p, nil
}

func enforcePrivateSend(tx *gorm.DB, msg *model.Message) error {
	pair, err := lockPrivatePair(tx, msg.FromID, msg.ToID)
	if err != nil {
		return err
	}
	// MySQL repeatable-read snapshots may predate a concurrent sender/friend
	// mutation. Current locking reads must observe the state behind the pair lock.
	p, err := ReadPrivatePolicy(tx.Clauses(clause.Locking{Strength: "UPDATE"}), msg.FromID, msg.ToID)
	if err != nil {
		return err
	}
	if !p.AddedByMe {
		return apierrors.New(apierrors.CodePrivateFriendRequired, "请先添加对方为好友")
	}
	if p.SendPolicy == SendUnrestricted {
		return nil
	}
	if msg.MsgType != model.MsgTypeText {
		return apierrors.New(apierrors.CodePrivateTextOnly, "互加好友前只能发送一条文本")
	}
	if p.IntroRemaining == 0 {
		return apierrors.New(apierrors.CodePrivateIntroUsed, "已发送首次文本，请等待对方添加好友")
	}
	column := "low_intro_used"
	if msg.FromID == pair.HighID {
		column = "high_intro_used"
	}
	return tx.Model(pair).Update(column, true).Error
}
