package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CommentThreadEntity struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	ConvID     uuid.UUID `gorm:"type:uuid;not null;index:idx_comment_threads_message_range,priority:1"`
	MessageID  uuid.UUID `gorm:"type:uuid;not null;index:idx_comment_threads_message_range,priority:2"`
	RangeStart int       `gorm:"not null;index:idx_comment_threads_message_range,priority:3"`
	RangeEnd   int       `gorm:"not null;index:idx_comment_threads_message_range,priority:4"`

	UserID uuid.UUID `gorm:"index;type:uuid;not null"`

	SelectedTextSnapshot string `gorm:"type:text;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (t *CommentThreadEntity) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
