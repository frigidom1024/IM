package service

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gochat/internal/logic/model"
	"gochat/internal/pkg/mysql"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGroupForbidden = errors.New("group permission denied")
	ErrGroupInvalid   = errors.New("invalid group operation")
	ErrGroupFull      = errors.New("group capacity reached")
)

// GroupAdminService serializes all membership and administration changes on
// the group row, so concurrent joins, transfers and dissolution remain atomic.
type GroupAdminService struct{}

func groupDB() (*gorm.DB, error) {
	if mysql.Client == nil {
		return nil, errors.New("database unavailable")
	}
	return mysql.Client, nil
}

func (s GroupAdminService) Create(name, avatar string, owner int64, capacity int) (*model.Group, error) {
	name = strings.TrimSpace(name)
	if owner <= 0 || name == "" || utf8.RuneCountInString(name) > 128 || len(avatar) > 255 || capacity < 1 || capacity > 10000 {
		return nil, ErrGroupInvalid
	}
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	group := &model.Group{Name: name, Avatar: avatar, OwnerID: owner, MaxMembers: capacity}
	err = db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, owner).Error; err != nil {
			return err
		}
		if err := tx.Create(group).Error; err != nil {
			return err
		}
		return tx.Create(&model.GroupMember{GroupID: group.ID, UserID: owner, Role: model.RoleOwner}).Error
	})
	return group, err
}

func (s GroupAdminService) Member(groupID, userID int64) (*model.GroupMember, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	var member model.GroupMember
	err = db.Joins("JOIN `group` ON `group`.id = group_member.group_id").Where("group_member.group_id = ? AND group_member.user_id = ?", groupID, userID).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGroupForbidden
	}
	return &member, err
}

func (s GroupAdminService) change(groupID, actor int64, fn func(*gorm.DB, *model.Group, *model.GroupMember) error) error {
	db, err := groupDB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var group model.Group
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, groupID).Error; err != nil {
			return err
		}
		var member model.GroupMember
		if err := tx.Where("group_id = ? AND user_id = ?", groupID, actor).First(&member).Error; err != nil {
			return ErrGroupForbidden
		}
		return fn(tx, &group, &member)
	})
}

func (s GroupAdminService) Join(groupID, userID int64) error {
	_, err := s.Apply(groupID, userID)
	return err
}

func addGroupMember(tx *gorm.DB, group *model.Group, userID int64) error {
	if userID <= 0 {
		return ErrGroupInvalid
	}
	var user model.User
	if err := tx.First(&user, userID).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&model.GroupMember{}).Where("group_id = ? AND user_id = ?", group.ID, userID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrGroupInvalid
	}
	if err := tx.Model(&model.GroupMember{}).Where("group_id = ?", group.ID).Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(group.MaxMembers) {
		return ErrGroupFull
	}
	if err := tx.Create(&model.GroupMember{GroupID: group.ID, UserID: userID, Role: model.RoleMember}).Error; err != nil {
		return err
	}
	return InitializeGroupConversation(tx, group.ID, userID)
}

func (s GroupAdminService) Invite(groupID, actor, target int64) error {
	return s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		if m.Role < model.RoleAdmin {
			return ErrGroupForbidden
		}
		if err := addGroupMember(tx, g, target); err != nil {
			return err
		}
		return tx.Model(&model.GroupApplication{}).Where("group_id=? AND user_id=? AND status='pending'", groupID, target).Updates(map[string]any{"status": "approved", "handled_by": actor, "handled_at": time.Now(), "reason": "邀请入群"}).Error
	})
}

func (s GroupAdminService) Remove(groupID, actor, target int64) error {
	return s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		var targetMember model.GroupMember
		if err := tx.Where("group_id = ? AND user_id = ?", groupID, target).First(&targetMember).Error; err != nil {
			return err
		}
		if targetMember.Role == model.RoleOwner || target == g.OwnerID {
			return ErrGroupForbidden
		}
		if target != actor && (m.Role < model.RoleAdmin || m.Role <= targetMember.Role) {
			return ErrGroupForbidden
		}
		if err := HideGroupConversations(tx, groupID, target); err != nil {
			return err
		}
		return tx.Delete(&targetMember).Error
	})
}

func (s GroupAdminService) Transfer(groupID, actor, target int64) error {
	return s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		if actor != g.OwnerID || m.Role != model.RoleOwner {
			return ErrGroupForbidden
		}
		if target == actor {
			return ErrGroupInvalid
		}
		var next model.GroupMember
		if err := tx.Where("group_id = ? AND user_id = ?", groupID, target).First(&next).Error; err != nil {
			return err
		}
		if err := tx.Model(g).Update("owner_id", target).Error; err != nil {
			return err
		}
		if err := tx.Model(m).Update("role", model.RoleMember).Error; err != nil {
			return err
		}
		return tx.Model(&next).Update("role", model.RoleOwner).Error
	})
}

func (s GroupAdminService) Announce(groupID, actor int64, text string) error {
	if utf8.RuneCountInString(text) > 4000 {
		return ErrGroupInvalid
	}
	return s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		if m.Role < model.RoleAdmin {
			return ErrGroupForbidden
		}
		return tx.Model(g).Update("announcement", text).Error
	})
}

func (s GroupAdminService) UpdateMember(groupID, actor, target int64, nickname *string, mutedUntil *time.Time, setMute bool, role *int8) error {
	if nickname != nil && utf8.RuneCountInString(*nickname) > 128 {
		return ErrGroupInvalid
	}
	if role != nil && (*role < model.RoleMember || *role > model.RoleAdmin) {
		return ErrGroupInvalid
	}
	return s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		var dest model.GroupMember
		if err := tx.Where("group_id = ? AND user_id = ?", groupID, target).First(&dest).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if nickname != nil {
			if actor != target && (m.Role < model.RoleAdmin || m.Role <= dest.Role) {
				return ErrGroupForbidden
			}
			updates["nickname"] = *nickname
		}
		if setMute {
			if actor == target || m.Role < model.RoleAdmin || m.Role <= dest.Role {
				return ErrGroupForbidden
			}
			updates["muted_until"] = mutedUntil
		}
		if role != nil {
			if actor != g.OwnerID || m.Role != model.RoleOwner || target == actor || dest.Role == model.RoleOwner {
				return ErrGroupForbidden
			}
			updates["role"] = *role
		}
		return tx.Model(&dest).Updates(updates).Error
	})
}

func (s GroupAdminService) Dissolve(groupID, actor int64) error {
	return s.change(groupID, actor, func(tx *gorm.DB, g *model.Group, m *model.GroupMember) error {
		if g.OwnerID != actor || m.Role != model.RoleOwner {
			return ErrGroupForbidden
		}
		if err := tx.Model(&model.GroupApplication{}).Where("group_id=? AND status='pending'", groupID).Updates(map[string]any{"status": "closed", "reason": "群组已解散", "handled_by": actor, "handled_at": time.Now()}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", groupID).Delete(&model.GroupMember{}).Error; err != nil {
			return err
		}
		if err := HideGroupConversations(tx, groupID, 0); err != nil {
			return err
		}
		return tx.Delete(g).Error
	})
}

func (s GroupAdminService) Search(query string, limit, offset int) ([]model.Group, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	groups := make([]model.Group, 0)
	query = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(query, "!", "!!"), "%", "!%"), "_", "!_")
	err = db.Where("name LIKE ? ESCAPE '!'", "%"+query+"%").Order("id DESC").Limit(limit).Offset(offset).Find(&groups).Error
	return groups, err
}

func (s GroupAdminService) Members(groupID, userID int64, limit, offset int) ([]model.GroupMember, error) {
	if _, err := s.Member(groupID, userID); err != nil {
		return nil, err
	}
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	members := make([]model.GroupMember, 0)
	err = db.Where("group_id = ?", groupID).Order("role DESC, id ASC").Limit(limit).Offset(offset).Find(&members).Error
	return members, err
}
