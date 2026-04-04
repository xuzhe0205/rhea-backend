package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemoryChunkEntity struct {
	ID              uuid.UUID        `gorm:"type:uuid;primaryKey"`
	DocumentID      uuid.UUID        `gorm:"type:uuid;index;not null"`
	UserID          uuid.UUID        `gorm:"type:uuid;index;not null"`
	ProjectID       *uuid.UUID       `gorm:"type:uuid;index"`
	ConversationID  *uuid.UUID       `gorm:"type:uuid;index"`
	SourceType      MemorySourceType `gorm:"type:text;index;not null"`
	ChunkIndex      int              `gorm:"not null"`
	Content         string           `gorm:"type:text;not null"`
	TokenCount      int              `gorm:"not null;default:0"`
	ImportanceScore float64          `gorm:"not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (m *MemoryChunkEntity) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
