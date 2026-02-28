package postgres

import (
	"context"
	"fmt"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
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
		ID:        entity.ID,
		Title:     entity.Title,
		LastMsgID: entity.LastMsgID,
		Summary:   entity.Summary,
		UserID:    entity.UserID,
	}, nil
}

// ListConversationsByUserID 获取用户的所有对话列表
func (s *PostgresStore) ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	var entities []model.ConversationEntity

	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
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
		}
	}
	return results, nil
}

func (s *PostgresStore) UpdateConversationStatus(ctx context.Context, convID string, newLastMsgID string, expectedOldMsgID *string, tokenDelta int) error {
	uNewID, _ := uuid.Parse(newLastMsgID)
	uConvID, _ := uuid.Parse(convID)

	db := s.db.WithContext(ctx).Table("conversation_entities").Where("id = ?", uConvID)

	if expectedOldMsgID != nil && *expectedOldMsgID != "" {
		uOldID, _ := uuid.Parse(*expectedOldMsgID)
		db = db.Where("last_msg_id = ?", uOldID)
	} else {
		db = db.Where("last_msg_id IS NULL")
	}

	result := db.Updates(map[string]interface{}{
		"last_msg_id": uNewID,
		"updated_at":  time.Now(),
	})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no rows updated (conflict or not found)")
	}
	return nil
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
