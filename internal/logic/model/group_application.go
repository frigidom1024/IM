package model

import "time"

type GroupApplication struct {
	ID            int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupID       int64      `gorm:"not null;index:idx_application_group_status,priority:1" json:"group_id"`
	UserID        int64      `gorm:"not null;index:idx_application_user" json:"user_id"`
	GroupName     string     `gorm:"size:128;not null" json:"group_name"`
	Status        string     `gorm:"size:16;not null;index:idx_application_group_status,priority:2" json:"status"`
	Reason        string     `gorm:"size:512" json:"reason"`
	HandledBy     int64      `json:"handled_by"`
	CreatedAt     time.Time  `json:"created_at"`
	HandledAt     *time.Time `json:"handled_at"`
	ApplicantName string     `gorm:"->;-:migration" json:"applicant_name"`
}

func (GroupApplication) TableName() string { return "group_application" }
