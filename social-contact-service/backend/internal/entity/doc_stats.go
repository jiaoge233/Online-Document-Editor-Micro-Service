package entity

import "time"

type DocStats struct {
	DocID             string `gorm:"primaryKey;type:varchar(64)"`
	LikeCount         uint64 `gorm:"default:0"`
	ViewCount         uint64 `gorm:"default:0"`
	ShareCount        uint64 `gorm:"default:0"`
	QuestionMarkCount uint64 `gorm:"default:0"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
