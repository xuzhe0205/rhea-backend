package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"rhea-backend/internal/model"
	"rhea-backend/internal/rag"
	"rhea-backend/internal/retrieval"
	"rhea-backend/internal/store"
)

type vectorSearchRow struct {
	ID              uuid.UUID
	DocumentID      uuid.UUID
	UserID          uuid.UUID
	ProjectID       *uuid.UUID
	ConversationID  *uuid.UUID
	SourceType      model.MemorySourceType
	ChunkIndex      int
	Content         string
	TokenCount      int
	ImportanceScore float64
	VectorScore     float64
}

type keywordSearchRow struct {
	ID              uuid.UUID
	DocumentID      uuid.UUID
	UserID          uuid.UUID
	ProjectID       *uuid.UUID
	ConversationID  *uuid.UUID
	SourceType      model.MemorySourceType
	ChunkIndex      int
	Content         string
	TokenCount      int
	ImportanceScore float64
	KeywordScore    float64
}

func (s *PostgresStore) CreateMemoryDocument(ctx context.Context, doc *model.MemoryDocumentEntity) error {
	if doc == nil {
		return fmt.Errorf("memory document is nil")
	}
	return s.db.WithContext(ctx).Create(doc).Error
}

func (s *PostgresStore) BulkCreateMemoryChunks(ctx context.Context, chunks []model.MemoryChunkEntity) error {
	if len(chunks) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&chunks).Error
}

func (s *PostgresStore) BulkCreateMemoryEmbeddings(ctx context.Context, embeddings []model.MemoryEmbeddingEntity) error {
	if len(embeddings) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&embeddings).Error
}

func (s *PostgresStore) UpdateConversationMemoryCheckpoint(
	ctx context.Context,
	conversationID uuid.UUID,
	checkpointMsgID uuid.UUID,
) error {
	now := time.Now()

	result := s.db.WithContext(ctx).
		Model(&model.ConversationEntity{}).
		Where("id = ?", conversationID).
		Updates(map[string]interface{}{
			"memory_checkpoint_msg_id": checkpointMsgID,
			"summary_updated_at":       &now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *PostgresStore) VectorSearchMemoryChunks(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
	projectID *uuid.UUID,
	scope rag.Scope,
	queryEmbedding []float32,
	limit int,
) ([]store.MemoryChunkSearchResult, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if limit <= 0 {
		limit = 8
	}

	embeddingLiteral := vectorLiteral(queryEmbedding)

	scopeClause := ""
	scopeArgs := make([]interface{}, 0, 2)

	switch scope {
	case rag.ScopeConversationOnly:
		scopeClause = "mc.conversation_id = ?"
		scopeArgs = append(scopeArgs, conversationID)

	case rag.ScopeConversationAndProject:
		if projectID == nil {
			return nil, fmt.Errorf("project_id is required for conversation_and_project scope")
		}
		scopeClause = "(mc.conversation_id = ? OR mc.project_id = ?)"
		scopeArgs = append(scopeArgs, conversationID, *projectID)

	case rag.ScopeProjectOnly:
		if projectID == nil {
			return nil, fmt.Errorf("project_id is required for project_only scope")
		}
		scopeClause = "mc.project_id = ?"
		scopeArgs = append(scopeArgs, *projectID)

	default:
		return nil, fmt.Errorf("unsupported retrieval scope: %s", scope)
	}

	baseSQL := `
	SELECT
		mc.id,
		mc.document_id,
		mc.user_id,
		mc.project_id,
		mc.conversation_id,
		mc.source_type,
		mc.chunk_index,
		mc.content,
		mc.token_count,
		mc.importance_score,
		1 - (me.embedding <=> ?::vector) AS vector_score
	FROM memory_chunk_entities mc
	JOIN memory_embedding_entities me
		ON me.chunk_id = mc.id
	JOIN memory_document_entities md
		ON md.id = mc.document_id
	WHERE
		mc.user_id = ?
		AND md.user_id = ?
		AND md.active = true
		AND md.status = 'indexed'
		AND ` + scopeClause + `
	ORDER BY me.embedding <=> ?::vector
	LIMIT ?
	`

	args := []interface{}{
		embeddingLiteral, // 1 - SELECT score
		userID,           // 2 - mc.user_id
		userID,           // 3 - md.user_id
	}
	args = append(args, scopeArgs...)     // 4... - scope clause
	args = append(args, embeddingLiteral) // next - ORDER BY distance
	args = append(args, limit)            // last - LIMIT

	var rows []vectorSearchRow
	if err := s.db.WithContext(ctx).Raw(baseSQL, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	results := make([]store.MemoryChunkSearchResult, 0, len(rows))
	for _, r := range rows {
		results = append(results, store.MemoryChunkSearchResult{
			Chunk: model.MemoryChunkEntity{
				ID:              r.ID,
				DocumentID:      r.DocumentID,
				UserID:          r.UserID,
				ProjectID:       r.ProjectID,
				ConversationID:  r.ConversationID,
				SourceType:      r.SourceType,
				ChunkIndex:      r.ChunkIndex,
				Content:         r.Content,
				TokenCount:      r.TokenCount,
				ImportanceScore: r.ImportanceScore,
			},
			VectorScore:  r.VectorScore,
			KeywordScore: 0,
		})
	}

	return results, nil
}

func (s *PostgresStore) KeywordSearchMemoryChunks(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
	projectID *uuid.UUID,
	scope rag.Scope,
	query string,
	ftsConfig string,
	limit int,
) ([]store.MemoryChunkSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []store.MemoryChunkSearchResult{}, nil
	}
	if limit <= 0 {
		limit = 8
	}

	cfg := retrieval.NormalizeFTSConfig(ftsConfig)

	scopeClause := ""
	scopeArgs := make([]interface{}, 0, 2)

	switch scope {
	case rag.ScopeConversationOnly:
		scopeClause = "mc.conversation_id = ?"
		scopeArgs = append(scopeArgs, conversationID)

	case rag.ScopeConversationAndProject:
		if projectID == nil {
			return nil, fmt.Errorf("project_id is required for conversation_and_project scope")
		}
		scopeClause = "(mc.conversation_id = ? OR mc.project_id = ?)"
		scopeArgs = append(scopeArgs, conversationID, *projectID)

	case rag.ScopeProjectOnly:
		if projectID == nil {
			return nil, fmt.Errorf("project_id is required for project_only scope")
		}
		scopeClause = "mc.project_id = ?"
		scopeArgs = append(scopeArgs, *projectID)

	default:
		return nil, fmt.Errorf("unsupported retrieval scope: %s", scope)
	}

	baseSQL := `
	SELECT
		mc.id,
		mc.document_id,
		mc.user_id,
		mc.project_id,
		mc.conversation_id,
		mc.source_type,
		mc.chunk_index,
		mc.content,
		mc.token_count,
		mc.importance_score,
		ts_rank_cd(
			to_tsvector('` + cfg + `', coalesce(mc.content, '')),
			websearch_to_tsquery('` + cfg + `', ?)
		) AS keyword_score
	FROM memory_chunk_entities mc
	JOIN memory_document_entities md
		ON md.id = mc.document_id
	WHERE
		mc.user_id = ?
		AND md.user_id = ?
		AND md.active = true
		AND md.status = 'indexed'
		AND ` + scopeClause + `
		AND to_tsvector('` + cfg + `', coalesce(mc.content, '')) @@ websearch_to_tsquery('` + cfg + `', ?)
	ORDER BY keyword_score DESC
	LIMIT ?
	`

	args := []interface{}{
		query,  // ts_rank_cd query
		userID, // mc.user_id
		userID, // md.user_id
	}
	args = append(args, scopeArgs...) // scope
	args = append(args, query)        // @@ query
	args = append(args, limit)        // limit

	var rows []keywordSearchRow
	if err := s.db.WithContext(ctx).Raw(baseSQL, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	results := make([]store.MemoryChunkSearchResult, 0, len(rows))
	for _, r := range rows {
		results = append(results, store.MemoryChunkSearchResult{
			Chunk: model.MemoryChunkEntity{
				ID:              r.ID,
				DocumentID:      r.DocumentID,
				UserID:          r.UserID,
				ProjectID:       r.ProjectID,
				ConversationID:  r.ConversationID,
				SourceType:      r.SourceType,
				ChunkIndex:      r.ChunkIndex,
				Content:         r.Content,
				TokenCount:      r.TokenCount,
				ImportanceScore: r.ImportanceScore,
			},
			VectorScore:  0,
			KeywordScore: r.KeywordScore,
		})
	}

	return results, nil
}

func (s *PostgresStore) MarkMemoryDocumentIndexed(
	ctx context.Context,
	documentID uuid.UUID,
) error {
	now := time.Now()

	result := s.db.WithContext(ctx).
		Model(&model.MemoryDocumentEntity{}).
		Where("id = ?", documentID).
		Updates(map[string]interface{}{
			"status":     model.MemoryDocIndexed,
			"active":     true,
			"indexed_at": &now,
			"failed_at":  nil,
			"error_msg":  "",
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *PostgresStore) MarkMemoryDocumentFailed(
	ctx context.Context,
	documentID uuid.UUID,
	errMsg string,
) error {
	now := time.Now()

	result := s.db.WithContext(ctx).
		Model(&model.MemoryDocumentEntity{}).
		Where("id = ?", documentID).
		Updates(map[string]interface{}{
			"status":    model.MemoryDocFailed,
			"active":    false,
			"failed_at": &now,
			"error_msg": errMsg,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *PostgresStore) DeactivateActiveMemoryDocuments(
	ctx context.Context,
	conversationID uuid.UUID,
	sourceType model.MemorySourceType,
	excludeDocumentID uuid.UUID,
) error {
	return s.db.WithContext(ctx).
		Model(&model.MemoryDocumentEntity{}).
		Where(
			"conversation_id = ? AND source_type = ? AND active = true AND id <> ?",
			conversationID,
			sourceType,
			excludeDocumentID,
		).
		Update("active", false).Error
}
