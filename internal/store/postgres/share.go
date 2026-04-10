package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"rhea-backend/internal/model"
)

func (s *PostgresStore) CreateShareLink(ctx context.Context, link *model.ShareLink) error {
	idsJSON, err := json.Marshal(link.MessageIDs)
	if err != nil {
		return fmt.Errorf("marshal message ids: %w", err)
	}
	entity := model.ShareLinkEntity{
		ID:            link.ID,
		Token:         link.Token,
		CreatorUserID: link.CreatorUserID,
		MessageIDs:    idsJSON,
		CreatedAt:     link.CreatedAt,
	}
	if err := s.db.WithContext(ctx).Create(&entity).Error; err != nil {
		return fmt.Errorf("create share link: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetShareLinkByToken(ctx context.Context, token string) (*model.ShareLink, error) {
	var entity model.ShareLinkEntity
	err := s.db.WithContext(ctx).
		Where("token = ? AND revoked_at IS NULL", token).
		First(&entity).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("get share link: %w", err)
	}
	return entity.ToDomain()
}

// GetMessagesByIDs fetches messages by their IDs in the order given.
// Ownership is NOT checked here — the share link creation enforces that.
func (s *PostgresStore) GetMessagesByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var entities []model.MessageEntity
	if err := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("get messages by ids: %w", err)
	}

	// Index by ID so we can return in the caller's requested order
	entityMap := make(map[uuid.UUID]model.MessageEntity, len(entities))
	for _, e := range entities {
		entityMap[e.ID] = e
	}

	msgs := make([]model.Message, 0, len(ids))
	for _, id := range ids {
		e, ok := entityMap[id]
		if !ok {
			continue
		}
		var meta map[string]interface{}
		if len(e.Metadata) > 0 {
			_ = json.Unmarshal(e.Metadata, &meta)
		}
		msgs = append(msgs, model.Message{
			ID:            e.ID,
			ConvID:        e.ConvID,
			Role:          e.Role,
			Content:       e.Content,
			InputTokens:   e.InputToken,
			OutputTokens:  e.OutputToken,
			CreatedAt:     e.CreatedAt,
			Metadata:      meta,
			IsFavorite:    e.IsFavorite,
			FavoriteLabel: e.FavoriteLabel,
		})
	}
	return msgs, nil
}

// RevokeShareLink marks a share link as revoked. Only the creator can do this
// (enforced at the handler layer).
func (s *PostgresStore) RevokeShareLink(ctx context.Context, token string, requesterUserID uuid.UUID) error {
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).
		Model(&model.ShareLinkEntity{}).
		Where("token = ? AND creator_user_id = ? AND revoked_at IS NULL", token, requesterUserID).
		Update("revoked_at", now)
	if result.Error != nil {
		return fmt.Errorf("revoke share link: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
