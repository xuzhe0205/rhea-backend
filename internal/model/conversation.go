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

	// --- 新增字段 ---
	Summary   string     `gorm:"type:text"`       // 存储滚动摘要
	LastMsgID *uuid.UUID `gorm:"type:uuid;index"` // 记录该对话当前指向的最后一条消息 ID
	// ----------------

	IsPinned  bool `gorm:"default:false"`
	TokenSum  int  `gorm:"default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c *ConversationEntity) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
