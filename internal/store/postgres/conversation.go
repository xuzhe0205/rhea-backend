package postgres

import (
	"context"
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
		ID:               entity.ID,
		Title:            entity.Title,
		LastMsgID:        entity.LastMsgID,
		Summary:          entity.Summary,
		UserID:           entity.UserID,
		IsPinned:         entity.IsPinned,
		PinnedAt:         entity.PinnedAt,
		CumulativeTokens: entity.TokenSum,
	}, nil
}

// ListConversationsByUserID 获取用户的所有对话列表
func (s *PostgresStore) ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	var entities []model.ConversationEntity

	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
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
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
