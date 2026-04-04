package retrieval

import (
	"rhea-backend/internal/model"
	"rhea-backend/internal/rag"

	"github.com/google/uuid"
)

type QueryInput struct {
	UserID         uuid.UUID
	ConversationID uuid.UUID
	ProjectID      *uuid.UUID
	Query          string
	TopK           int
	Scope          rag.Scope
}

type RetrievedChunk struct {
	Chunk        model.MemoryChunkEntity
	VectorScore  float64
	KeywordScore float64
	FinalScore   float64
}

type RetrievedContext struct {
	Chunks []RetrievedChunk
}
