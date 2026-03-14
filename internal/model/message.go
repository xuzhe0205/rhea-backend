package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type MessageEntity struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ConvID      uuid.UUID      `gorm:"index;type:uuid"`
	Role        Role           `gorm:"size:20"` // 直接存 string 或自定义 Role 类型
	Content     string         `gorm:"type:text"`
	Tokens      int            `gorm:"default:0"`
	ParentMsgID *uuid.UUID     `gorm:"index;type:uuid"` // 树状结构
	IsPinned    bool           `gorm:"default:false"`
	Metadata    datatypes.JSON `gorm:"type:jsonb"`
	InputToken  int            `gorm:"default:0"`
	OutputToken int            `gorm:"default:0"`
	CreatedAt   time.Time
}

func (m *MessageEntity) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
