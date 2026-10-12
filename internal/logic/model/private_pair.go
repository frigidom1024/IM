package model

// PrivatePair survives removal of either directed friend edge. All friend
// mutations and private sends serialize on this canonical, ordered user pair.
type PrivatePair struct {
	LowID         int64 `gorm:"primaryKey;autoIncrement:false"`
	HighID        int64 `gorm:"primaryKey;autoIncrement:false"`
	LowIntroUsed  bool  `gorm:"not null;default:false"`
	HighIntroUsed bool  `gorm:"not null;default:false"`
}

func (PrivatePair) TableName() string { return "private_pair" }
