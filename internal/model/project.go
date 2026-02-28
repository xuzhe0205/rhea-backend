package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Project struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;index;not null"` // 属于哪个用户
	Name      string    `gorm:"size:255;not null"`
	Summary   string    `gorm:"type:text"` // 预留给 P4 的自动摘要
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
