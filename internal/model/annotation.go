package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AnnotationType string

// 2. 定义常量 (枚举值)
const (
	TypeHighlight AnnotationType = "highlight"
	TypeUnderline AnnotationType = "underline"
	TypeBold      AnnotationType = "bold"
	TypeComment   AnnotationType = "comment"
	TypeFavorite  AnnotationType = "favorite"
)

type AnnotationEntity struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	MessageID   uuid.UUID      `gorm:"index;type:uuid"`
	ConvID      uuid.UUID      `gorm:"index;type:uuid"`
	UserID      uuid.UUID      `gorm:"index;type:uuid"`
	Type        AnnotationType `gorm:"size:20"` // enum: highlight, comment, favorite, bold, etc.
	RangeStart  int
	RangeEnd    int
	UserNote    string         `gorm:"type:text"`
	StyleConfig datatypes.JSON `gorm:"type:jsonb"` // 存储 {"color": "#FF5733"} 等
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ann *AnnotationEntity) BeforeCreate(tx *gorm.DB) error {
	if ann.ID == uuid.Nil {
		ann.ID = uuid.New()
	}
	return nil
}
