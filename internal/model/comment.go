package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CommentEntity struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// 归属 thread
	ThreadID uuid.UUID `gorm:"index;type:uuid;not null"`

	// 归属上下文
	ConvID    uuid.UUID `gorm:"index;type:uuid;not null"`
	MessageID uuid.UUID `gorm:"index;type:uuid;not null"`
	UserID    uuid.UUID `gorm:"index;type:uuid;not null"` // comment 作者

	Content string `gorm:"type:text;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (c *CommentEntity) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
