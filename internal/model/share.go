package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ShareLink is the domain model for a shared message link.
type ShareLink struct {
	ID            uuid.UUID   `json:"id"`
	Token         string      `json:"token"`
	CreatorUserID uuid.UUID   `json:"-"` // never exposed to viewers
	MessageIDs    []uuid.UUID `json:"message_ids"`
	CreatedAt     time.Time   `json:"created_at"`
	RevokedAt     *time.Time  `json:"revoked_at,omitempty"`
}

// ShareLinkEntity is the GORM model for the share_links table.
type ShareLinkEntity struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Token         string     `gorm:"uniqueIndex;not null;size:32"`
	CreatorUserID uuid.UUID  `gorm:"type:uuid;not null;index"`
	MessageIDs    []byte     `gorm:"type:jsonb;not null"`
	CreatedAt     time.Time  `gorm:"not null"`
	RevokedAt     *time.Time
}

func (ShareLinkEntity) TableName() string { return "share_links" }

func (e *ShareLinkEntity) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

func (e *ShareLinkEntity) ToDomain() (*ShareLink, error) {
	var ids []uuid.UUID
	if err := json.Unmarshal(e.MessageIDs, &ids); err != nil {
		return nil, fmt.Errorf("unmarshal message ids: %w", err)
	}
	return &ShareLink{
		ID:            e.ID,
		Token:         e.Token,
		CreatorUserID: e.CreatorUserID,
		MessageIDs:    ids,
		CreatedAt:     e.CreatedAt,
		RevokedAt:     e.RevokedAt,
	}, nil
}
