package model

import "time"

type UploadSession struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	UserID         int64      `json:"-"`
	GroupID        int64      `json:"group_id"`
	Name           string     `gorm:"size:255" json:"name"`
	Type           string     `gorm:"size:20" json:"type"`
	MIME           string     `gorm:"size:128" json:"mime"`
	Size           int64      `json:"size"`
	Offset         int64      `json:"offset"`
	TempPath       string     `gorm:"size:1024" json:"-"`
	AssetID        string     `gorm:"size:64" json:"asset_id,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AssetExpiresAt *time.Time `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (UploadSession) TableName() string { return "upload_session" }

type MediaAsset struct {
	ID            string     `gorm:"primaryKey;size:64" json:"id"`
	UserID        int64      `json:"owner_id"`
	GroupID       int64      `json:"group_id"`
	ObjectPath    string     `gorm:"size:512" json:"-"`
	ThumbnailPath string     `gorm:"size:512" json:"-"`
	Name          string     `gorm:"size:255" json:"name"`
	MIME          string     `gorm:"size:128" json:"mime"`
	Size          int64      `json:"size"`
	Width         int        `json:"width,omitempty"`
	Height        int        `json:"height,omitempty"`
	Duration      float64    `json:"duration,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (MediaAsset) TableName() string { return "media_asset" }
