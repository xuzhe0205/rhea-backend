package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConversationEntity struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID  `gorm:"type:uuid;index;not null"`
	ProjectID *uuid.UUID `gorm:"type:uuid;index"`
	Title     string     `gorm:"size:255"`

	Summary   string     `gorm:"type:text"`
	LastMsgID *uuid.UUID `gorm:"type:uuid;index"`

	IsPinned  bool       `gorm:"default:false"`
	PinnedAt  *time.Time `gorm:"index"`
	TokenSum  int        `gorm:"default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c *ConversationEntity) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
