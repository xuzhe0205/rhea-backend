package ingest

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"rhea-backend/internal/embedding"
	"rhea-backend/internal/model"
	"rhea-backend/internal/store"
)

type ConversationIngestor struct {
	Store      store.Store
	Embeddings *embedding.Service
}

func (i *ConversationIngestor) RebuildConversationSnapshot(
	ctx context.Context,
	conversationID string,
) (err error) {
	if i == nil {
		return fmt.Errorf("conversation ingestor is nil")
	}
	if i.Store == nil {
		return fmt.Errorf("store is nil")
	}
	if i.Embeddings == nil {
		return fmt.Errorf("embedding service is nil")
	}

	conv, err := i.Store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	msgs, err := i.Store.GetAllMessagesByConvID(ctx, conversationID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	rawText := buildConversationRawText(msgs)
	if strings.TrimSpace(rawText) == "" {
		return nil
	}

	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return err
	}

	doc := &model.MemoryDocumentEntity{
		UserID:         conv.UserID,
		ProjectID:      conv.ProjectID,
		ConversationID: &convUUID,
		SourceType:     model.MemorySourceConversationSegment,
		Title:          conv.Title,
		Status:         model.MemoryDocPending,
		Active:         false,
	}

	if err := i.Store.CreateMemoryDocument(ctx, doc); err != nil {
		return err
	}

	defer func() {
		if err != nil && doc != nil && doc.ID != uuid.Nil {
			if statusErr := i.Store.MarkMemoryDocumentFailed(ctx, doc.ID, err.Error()); statusErr != nil {
				log.Printf("[Ingest] Failed to mark document failed. doc=%s err=%v", doc.ID, statusErr)
			}
		}
	}()

	chunkTexts := chunkText(rawText, 1200, 150)
	if len(chunkTexts) == 0 {
		if err := i.Store.MarkMemoryDocumentIndexed(ctx, doc.ID); err != nil {
			return err
		}

		if err := i.Store.DeactivateActiveMemoryDocuments(
			ctx,
			convUUID,
			model.MemorySourceConversationSegment,
			doc.ID,
		); err != nil {
			return err
		}

		return nil
	}

	chunks := make([]model.MemoryChunkEntity, 0, len(chunkTexts))
	texts := make([]string, 0, len(chunkTexts))

	for idx, c := range chunkTexts {
		chunks = append(chunks, model.MemoryChunkEntity{
			DocumentID:     doc.ID,
			UserID:         conv.UserID,
			ProjectID:      conv.ProjectID,
			ConversationID: &convUUID,
			SourceType:     model.MemorySourceConversationSegment,
			ChunkIndex:     idx,
			Content:        c,
			TokenCount:     0,
		})
		texts = append(texts, c)
	}

	if err := i.Store.BulkCreateMemoryChunks(ctx, chunks); err != nil {
		return err
	}

	vectors, err := i.Embeddings.EmbedTexts(ctx, texts)
	if err != nil {
		return err
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(chunks))
	}

	embeddings := make([]model.MemoryEmbeddingEntity, 0, len(vectors))
	for idx, vec := range vectors {
		embeddings = append(embeddings, model.MemoryEmbeddingEntity{
			ChunkID:        chunks[idx].ID,
			Embedding:      pgvector.NewVector(vec),
			EmbeddingModel: i.Embeddings.Provider.ModelName(),
		})
	}

	if err := i.Store.BulkCreateMemoryEmbeddings(ctx, embeddings); err != nil {
		return err
	}

	lastMsgID := msgs[len(msgs)-1].ID
	if lastMsgID != uuid.Nil {
		if err := i.Store.UpdateConversationMemoryCheckpoint(ctx, convUUID, lastMsgID); err != nil {
			return err
		}
	}

	if err := i.Store.MarkMemoryDocumentIndexed(ctx, doc.ID); err != nil {
		return err
	}

	if err := i.Store.DeactivateActiveMemoryDocuments(
		ctx,
		convUUID,
		model.MemorySourceConversationSegment,
		doc.ID,
	); err != nil {
		return err
	}

	return nil
}
