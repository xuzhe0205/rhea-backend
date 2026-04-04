package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConversationEntity struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID                uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProjectID             *uuid.UUID `gorm:"type:uuid;index"`
	Title                 string     `gorm:"size:255;not null"`
	Summary               string     `gorm:"type:text"`
	LastMsgID             *uuid.UUID `gorm:"type:uuid"`
	IsPinned              bool       `gorm:"not null;default:false;index"`
	PinnedAt              *time.Time `gorm:"index"`
	TokenSum              int        `gorm:"not null;default:0"`
	SummaryUpdatedAt      *time.Time `gorm:"type:timestamptz"`
	MemoryCheckpointMsgID *uuid.UUID `gorm:"type:uuid"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (c *ConversationEntity) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
