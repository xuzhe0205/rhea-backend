package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemorySourceType string
type MemoryDocStatus string

const (
	MemorySourceConversationSummary MemorySourceType = "conversation_summary"
	MemorySourceConversationSegment MemorySourceType = "conversation_segment"
	MemorySourceProjectNote         MemorySourceType = "project_note"
	MemorySourceUploadedFile        MemorySourceType = "uploaded_file"

	MemoryDocPending MemoryDocStatus = "pending"
	MemoryDocIndexed MemoryDocStatus = "indexed"
	MemoryDocFailed  MemoryDocStatus = "failed"
	MemoryDocDeleted MemoryDocStatus = "deleted"
)

type MemoryDocumentEntity struct {
	ID             uuid.UUID        `gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID        `gorm:"type:uuid;index;not null"`
	ProjectID      *uuid.UUID       `gorm:"type:uuid;index"`
	ConversationID *uuid.UUID       `gorm:"type:uuid;index"`
	SourceType     MemorySourceType `gorm:"type:text;index;not null"`
	SourceRefID    *uuid.UUID       `gorm:"type:uuid;index"`
	Title          string           `gorm:"type:text"`
	Status         MemoryDocStatus  `gorm:"type:text;index;not null;default:'pending'"`
	Active         bool             `gorm:"not null;default:false;index"`
	IndexedAt      *time.Time       `gorm:"type:timestamptz"`
	FailedAt       *time.Time       `gorm:"type:timestamptz"`
	ErrorMsg       string           `gorm:"type:text"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (m *MemoryDocumentEntity) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
