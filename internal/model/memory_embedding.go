package model

import (
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

type MemoryEmbeddingEntity struct {
	ChunkID        uuid.UUID       `gorm:"type:uuid;primaryKey"`
	Embedding      pgvector.Vector `gorm:"type:vector(1536);not null"`
	EmbeddingModel string          `gorm:"type:text;not null"`
	CreatedAt      time.Time
}
