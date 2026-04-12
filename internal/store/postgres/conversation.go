package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateConversation(ctx context.Context, conv *model.Conversation) (string, error) {
	if conv.ID == uuid.Nil {
		conv.ID = uuid.New()
	}

	entity := &model.ConversationEntity{
		ID:        conv.ID,
		UserID:    conv.UserID,
		ProjectID: conv.ProjectID,
		Title:     conv.Title,
		Summary:   conv.Summary,
		LastMsgID: conv.LastMsgID,
	}

	if err := s.db.WithContext(ctx).Create(&entity).Error; err != nil {
		return "", fmt.Errorf("failed to create conversation: %w", err)
	}

	return entity.ID.String(), nil
}

func (s *PostgresStore) GetConversation(ctx context.Context, id string) (*model.Conversation, error) {
	var entity model.ConversationEntity
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&entity).Error; err != nil {
		return nil, err
	}

	return &model.Conversation{
		ID:                    entity.ID,
		Title:                 entity.Title,
		LastMsgID:             entity.LastMsgID,
		Summary:               entity.Summary,
		UserID:                entity.UserID,
		IsPinned:              entity.IsPinned,
		PinnedAt:              entity.PinnedAt,
		CumulativeTokens:      entity.TokenSum,
		ProjectID:             entity.ProjectID,
		CreatedAt:             entity.CreatedAt,
		UpdatedAt:             entity.UpdatedAt,
		SummaryUpdatedAt:      entity.SummaryUpdatedAt,
		MemoryCheckpointMsgID: entity.MemoryCheckpointMsgID,
	}, nil
}

// ListConversationsByUserID 获取用户的所有对话列表
func (s *PostgresStore) ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	var entities []model.ConversationEntity

	err := s.db.WithContext(ctx).
		Where("user_id = ? AND project_id IS NULL", userID).
		Order("is_pinned DESC").
		Order("pinned_at DESC NULLS LAST").
		Order("updated_at DESC").
		Find(&entities).Error

	if err != nil {
		return nil, err
	}

	results := make([]*model.Conversation, len(entities))
	for i, e := range entities {
		results[i] = &model.Conversation{
			ID:        e.ID,
			UserID:    e.UserID,
			Title:     e.Title,
			Summary:   e.Summary,
			LastMsgID: e.LastMsgID,
			IsPinned:  e.IsPinned,
			PinnedAt:  e.PinnedAt,
		}
	}
	return results, nil
}

func (s *PostgresStore) UpdateConversationStatus(ctx context.Context, convID string, newLastMsgID string, _ *string, tokenDelta int) (int, error) {
	uNewID, _ := uuid.Parse(newLastMsgID)
	uConvID, _ := uuid.Parse(convID)
	var updatedTokens int

	// 直接执行更新，不再校验旧的 last_msg_id
	err := s.db.WithContext(ctx).Table("conversation_entities").
		Where("id = ?", uConvID).
		Select("token_sum").
		Updates(map[string]interface{}{
			"last_msg_id": uNewID,
			"token_sum":   gorm.Expr("token_sum + ?", tokenDelta),
			"updated_at":  time.Now(),
		}).
		Scan(&updatedTokens).Error

	if err != nil {
		return 0, err
	}

	return updatedTokens, nil
}

func (s *PostgresStore) UpdateConversationTitle(ctx context.Context, convID string, title string) error {
	uID, _ := uuid.Parse(convID)
	result := s.db.WithContext(ctx).Model(&model.ConversationEntity{}).
		Where("id = ?", uID).
		Updates(map[string]interface{}{
			"title":      title,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *PostgresStore) SetSummary(ctx context.Context, conversationID string, summary string) error {
	return s.db.WithContext(ctx).Model(&model.ConversationEntity{}).
		Where("id = ?", conversationID).
		Update("summary", summary).Error
}

func (s *PostgresStore) GetSummary(ctx context.Context, conversationID string) (string, error) {
	var summary string
	err := s.db.WithContext(ctx).Model(&model.ConversationEntity{}).
		Select("summary").
		Where("id = ?", conversationID).
		Scan(&summary).Error
	return summary, err
}

func (s *PostgresStore) IncrementConversationTokenUsage(ctx context.Context, convID string, delta int) error {
	uID, _ := uuid.Parse(convID)
	return s.db.WithContext(ctx).Model(&model.ConversationEntity{}).
		Where("id = ?", uID).
		Update("token_sum", gorm.Expr("token_sum + ?", delta)).Error
}

func (s *PostgresStore) SetConversationPinned(ctx context.Context, convID string, isPinned bool) error {
	uID, err := uuid.Parse(convID)
	if err != nil {
		return fmt.Errorf("invalid conversation id: %w", err)
	}

	updates := map[string]interface{}{
		"is_pinned": isPinned,
	}

	if isPinned {
		now := time.Now()
		updates["pinned_at"] = &now
	} else {
		updates["pinned_at"] = nil
	}

	result := s.db.WithContext(ctx).
		Model(&model.ConversationEntity{}).
		Where("id = ?", uID).
		UpdateColumns(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteConversation cascades: annotations, comment threads+comments, messages, memory docs/chunks, then conversation.
// Returns the R2 image keys stored in message metadata so the caller can delete them from object storage.
func (s *PostgresStore) DeleteConversation(ctx context.Context, convID uuid.UUID) (imageKeys []string, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Collect R2 image keys from message metadata before deletion.
		var messages []model.MessageEntity
		if err := tx.Where("conv_id = ?", convID).Find(&messages).Error; err != nil {
			return err
		}

		for _, m := range messages {
			if len(m.Metadata) == 0 {
				continue
			}
			var meta map[string]interface{}
			if jsonErr := json.Unmarshal(m.Metadata, &meta); jsonErr != nil {
				continue
			}
			if raw, ok := meta["image_keys"]; ok {
				switch v := raw.(type) {
				case []string:
					imageKeys = append(imageKeys, v...)
				case []interface{}:
					for _, item := range v {
						if s, ok := item.(string); ok {
							imageKeys = append(imageKeys, s)
						}
					}
				}
			}
		}

		// 2. Delete annotations (linked via message_id → messages in this conversation).
		if err := tx.
			Where("conv_id = ?", convID).
			Delete(&model.AnnotationEntity{}).Error; err != nil {
			return err
		}

		// 3. Delete comments belonging to threads of this conversation.
		if err := tx.
			Where("thread_id IN (SELECT id FROM comment_thread_entities WHERE conv_id = ?)", convID).
			Delete(&model.CommentEntity{}).Error; err != nil {
			return err
		}

		// 4. Delete comment threads.
		if err := tx.
			Where("conv_id = ?", convID).
			Delete(&model.CommentThreadEntity{}).Error; err != nil {
			return err
		}

		// 5. Delete memory chunks/embeddings via documents.
		var docIDs []uuid.UUID
		if err := tx.
			Model(&model.MemoryDocumentEntity{}).
			Where("conversation_id = ?", convID).
			Pluck("id", &docIDs).Error; err != nil {
			return err
		}
		if len(docIDs) > 0 {
			// Collect chunk IDs for embedding deletion.
			var chunkIDs []uuid.UUID
			if err := tx.
				Model(&model.MemoryChunkEntity{}).
				Where("document_id IN ?", docIDs).
				Pluck("id", &chunkIDs).Error; err != nil {
				return err
			}
			if len(chunkIDs) > 0 {
				if err := tx.Where("chunk_id IN ?", chunkIDs).Delete(&model.MemoryEmbeddingEntity{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&model.MemoryChunkEntity{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&model.MemoryDocumentEntity{}).Error; err != nil {
				return err
			}
		}

		// 6. Delete messages.
		if err := tx.Where("conv_id = ?", convID).Delete(&model.MessageEntity{}).Error; err != nil {
			return err
		}

		// 7. Delete the conversation itself.
		if err := tx.Where("id = ?", convID).Delete(&model.ConversationEntity{}).Error; err != nil {
			return err
		}

		return nil
	})
	return imageKeys, err
}

func (s *PostgresStore) ListPinnedConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	var entities []model.ConversationEntity

	err := s.db.WithContext(ctx).
		Where("user_id = ? AND is_pinned = ?", userID, true).
		Order("pinned_at DESC").
		Find(&entities).Error
	if err != nil {
		return nil, err
	}

	results := make([]*model.Conversation, len(entities))
	for i, e := range entities {
		results[i] = &model.Conversation{
			ID:        e.ID,
			UserID:    e.UserID,
			Title:     e.Title,
			Summary:   e.Summary,
			LastMsgID: e.LastMsgID,
			IsPinned:  e.IsPinned,
			PinnedAt:  e.PinnedAt,
		}
	}
	return results, nil
}
