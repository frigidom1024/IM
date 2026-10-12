package service

import (
	"errors"
	"gochat/internal/logic/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"unicode/utf8"
)

func (s GroupAdminService) ListGroups(user int64) ([]model.Group, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	items := make([]model.Group, 0)
	err = db.Raw("SELECT g.*,gm.role,(SELECT COUNT(*) FROM group_member m WHERE m.group_id=g.id) member_count,CASE WHEN g.owner_id=? THEN (SELECT COUNT(*) FROM group_application a WHERE a.group_id=g.id AND a.status='pending') ELSE 0 END pending_count FROM `group` g JOIN group_member gm ON gm.group_id=g.id WHERE gm.user_id=? ORDER BY g.id DESC", user, user).Scan(&items).Error
	return items, err
}

func (s GroupAdminService) Apply(groupID, userID int64) (*model.GroupApplication, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	var result model.GroupApplication
	err = db.Transaction(func(tx *gorm.DB) error {
		var group model.Group
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, groupID).Error; err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		var members int64
		if err := tx.Model(&model.GroupMember{}).Where("group_id=? AND user_id=?", groupID, userID).Count(&members).Error; err != nil {
			return err
		}
		if members > 0 {
			return ErrGroupInvalid
		}
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("group_id=? AND user_id=? AND status='pending'", groupID, userID).First(&result).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result = model.GroupApplication{GroupID: groupID, UserID: userID, GroupName: group.Name, Status: "pending"}
		return tx.Create(&result).Error
	})
	return &result, err
}

func (s GroupAdminService) Applications(groupID, actor int64, limit, offset int) ([]model.GroupApplication, error) {
	member, err := s.Member(groupID, actor)
	if err != nil {
		return nil, err
	}
	if member.Role != model.RoleOwner {
		return nil, ErrGroupForbidden
	}
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	rows := make([]model.GroupApplication, 0)
	err = db.Table("group_application a").Select("a.*,COALESCE(NULLIF(u.nickname,''),u.username) applicant_name").Joins("JOIN user u ON u.id=a.user_id").Where("a.group_id=?", groupID).Order("CASE WHEN a.status='pending' THEN 0 ELSE 1 END,a.id DESC").Limit(limit).Offset(offset).Scan(&rows).Error
	return rows, err
}
func (s GroupAdminService) MyApplications(userID int64, limit, offset int) ([]model.GroupApplication, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	rows := make([]model.GroupApplication, 0)
	err = db.Where("user_id=?", userID).Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, err
}
func (s GroupAdminService) Review(groupID, actor, applicationID int64, decision, reason string) (*model.GroupApplication, error) {
	if (decision != "approve" && decision != "reject") || utf8.RuneCountInString(reason) > 512 {
		return nil, ErrGroupInvalid
	}
	var result model.GroupApplication
	err := s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		if actor != g.OwnerID || m.Role != model.RoleOwner {
			return ErrGroupForbidden
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&result, "id=? AND group_id=?", applicationID, groupID).Error; err != nil {
			return err
		}
		status := "rejected"
		if decision == "approve" {
			status = "approved"
		}
		if result.Status == status {
			return nil
		}
		if result.Status != "pending" {
			return ErrGroupInvalid
		}
		if decision == "approve" {
			if err := addGroupMember(tx, g, result.UserID); err != nil {
				return err
			}
		}
		now := time.Now()
		result.Status = status
		result.Reason = reason
		result.HandledBy = actor
		result.HandledAt = &now
		return tx.Save(&result).Error
	})
	return &result, err
}
