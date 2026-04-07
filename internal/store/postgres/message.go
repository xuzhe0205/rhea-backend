package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (s *PostgresStore) AppendMessage(ctx context.Context, conversationID string, parentID *string, msg model.Message, metadata map[string]interface{}) (string, error) {
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return "", fmt.Errorf("invalid conversation id: %w", err)
	}

	var count int64
	err = s.db.WithContext(ctx).Model(&model.ConversationEntity{}).Where("id = ?", convUUID).Count(&count).Error
	if err != nil || count == 0 {
		return "", fmt.Errorf("conversation not found")
	}

	newMsgID := uuid.New()
	var pID *uuid.UUID
	if parentID != nil && *parentID != "" {
		parsedPID, err := uuid.Parse(*parentID)
		if err == nil {
			pID = &parsedPID
		}
	}

	var metaDataJSON datatypes.JSON
	if metadata != nil {
		b, _ := json.Marshal(metadata)
		metaDataJSON = datatypes.JSON(b)
	}

	dbMsg := model.MessageEntity{
		ID:          newMsgID,
		ConvID:      convUUID,
		ParentMsgID: pID,
		Role:        msg.Role,
		Content:     msg.Content,
		Metadata:    metaDataJSON,
		CreatedAt:   time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(&dbMsg).Error; err != nil {
		return "", fmt.Errorf("failed to persist message: %w", err)
	}

	return newMsgID.String(), nil
}

func (s *PostgresStore) GetMessagesByConvID(ctx context.Context, conversationID string, limit int, order string, beforeID string) ([]model.Message, error) {
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, fmt.Errorf("invalid conversation uuid: %w", err)
	}

	query := s.db.WithContext(ctx).Where("conv_id = ?", convUUID)

	// Cursor pagination based on (created_at, id)
	if beforeID != "" {
		var beforeMsg model.MessageEntity
		if err := s.db.WithContext(ctx).
			Select("id, created_at").
			First(&beforeMsg, "id = ?", beforeID).Error; err != nil {
			return nil, fmt.Errorf("before_id not found: %w", err)
		}

		query = query.Where(
			"(created_at < ?) OR (created_at = ? AND id < ?)",
			beforeMsg.CreatedAt,
			beforeMsg.CreatedAt,
			beforeMsg.ID,
		)
	}

	// Stable ordering
	query = query.Order("created_at desc, id desc")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var dbMsgs []model.MessageEntity
	if err := query.Find(&dbMsgs).Error; err != nil {
		return nil, err
	}

	msgs := make([]model.Message, len(dbMsgs))
	for i, m := range dbMsgs {
		var meta map[string]interface{}
		if len(m.Metadata) > 0 {
			_ = json.Unmarshal(m.Metadata, &meta)
		}

		msgs[i] = model.Message{
			ID:            m.ID,
			ConvID:        m.ConvID,
			Role:          m.Role,
			Content:       m.Content,
			CreatedAt:     m.CreatedAt,
			Metadata:      meta,
			IsFavorite:    m.IsFavorite,
			FavoriteLabel: m.FavoriteLabel,
		}
	}

	// Reverse for ascending display order
	if order == "asc" {
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
	}

	return msgs, nil
}

func (s *PostgresStore) GetMessagesForFavoriteJump(
	ctx context.Context,
	conversationID string,
	messageID string,
	olderBuffer int,
) ([]model.Message, error) {
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, fmt.Errorf("invalid conversation uuid: %w", err)
	}

	msgUUID, err := uuid.Parse(messageID)
	if err != nil {
		return nil, fmt.Errorf("invalid message uuid: %w", err)
	}

	var anchor model.MessageEntity
	if err := s.db.WithContext(ctx).
		Select("id, conv_id, created_at").
		Where("id = ? AND conv_id = ?", msgUUID, convUUID).
		First(&anchor).Error; err != nil {
		return nil, fmt.Errorf("favorite message not found in conversation: %w", err)
	}

	// Part 1: fetch up to olderBuffer messages BEFORE the favorite
	var older []model.MessageEntity
	if olderBuffer > 0 {
		if err := s.db.WithContext(ctx).
			Where("conv_id = ?", convUUID).
			Where(
				"(created_at < ?) OR (created_at = ? AND id < ?)",
				anchor.CreatedAt, anchor.CreatedAt, anchor.ID,
			).
			Order("created_at desc, id desc").
			Limit(olderBuffer).
			Find(&older).Error; err != nil {
			return nil, err
		}

		// reverse older so final output is asc
		for i, j := 0, len(older)-1; i < j; i, j = i+1, j-1 {
			older[i], older[j] = older[j], older[i]
		}
	}

	// Part 2: fetch favorite -> latest
	var newer []model.MessageEntity
	if err := s.db.WithContext(ctx).
		Where("conv_id = ?", convUUID).
		Where(
			"(created_at > ?) OR (created_at = ? AND id >= ?)",
			anchor.CreatedAt, anchor.CreatedAt, anchor.ID,
		).
		Order("created_at asc, id asc").
		Find(&newer).Error; err != nil {
		return nil, err
	}

	combined := make([]model.MessageEntity, 0, len(older)+len(newer))
	combined = append(combined, older...)
	combined = append(combined, newer...)

	msgs := make([]model.Message, len(combined))
	for i, m := range combined {
		var meta map[string]interface{}
		if len(m.Metadata) > 0 {
			_ = json.Unmarshal(m.Metadata, &meta)
		}

		msgs[i] = model.Message{
			ID:            m.ID,
			ConvID:        m.ConvID,
			Role:          m.Role,
			Content:       m.Content,
			CreatedAt:     m.CreatedAt,
			Metadata:      meta,
			IsFavorite:    m.IsFavorite,
			FavoriteLabel: m.FavoriteLabel,
		}
	}

	return msgs, nil
}

func (s *PostgresStore) SetMessageFavorite(
	ctx context.Context,
	messageID string,
	isFavorite bool,
) error {
	msgUUID, err := uuid.Parse(messageID)
	if err != nil {
		return fmt.Errorf("invalid message uuid: %w", err)
	}

	updates := map[string]interface{}{
		"is_favorite": isFavorite,
	}

	if isFavorite {
		now := time.Now()
		updates["favorited_at"] = &now
	} else {
		updates["favorited_at"] = nil
	}

	result := s.db.WithContext(ctx).
		Model(&model.MessageEntity{}).
		Where("id = ?", msgUUID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update favorite status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("message not found")
	}

	return nil
}

func (s *PostgresStore) ListFavoriteMessages(
	ctx context.Context,
	userID string,
	limit int,
	offset int,
) ([]model.FavoriteMessageRow, error) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user uuid: %w", err)
	}

	var rows []model.FavoriteMessageRow
	query := s.db.WithContext(ctx).
		Table("message_entities AS m").
		Select(`
			m.id AS id,
			m.conv_id AS conv_id,
			c.project_id AS project_id,
			m.role AS role,
			m.content AS content,
			m.created_at AS created_at,
			m.favorited_at AS favorited_at,
			m.favorite_label AS favorite_label
		`).
		Joins("JOIN conversation_entities AS c ON c.id = m.conv_id").
		Where("c.user_id = ? AND m.is_favorite = ?", userUUID, true).
		Order("m.favorited_at desc, m.created_at desc, m.id desc")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func (s *PostgresStore) GetMessageByID(ctx context.Context, messageID string) (*model.Message, error) {
	msgUUID, err := uuid.Parse(messageID)
	if err != nil {
		return nil, fmt.Errorf("invalid message uuid: %w", err)
	}

	var dbMsg model.MessageEntity
	if err := s.db.WithContext(ctx).
		First(&dbMsg, "id = ?", msgUUID).Error; err != nil {
		return nil, err
	}

	var meta map[string]interface{}
	if len(dbMsg.Metadata) > 0 {
		_ = json.Unmarshal(dbMsg.Metadata, &meta)
	}

	return &model.Message{
		ID:            dbMsg.ID,
		ConvID:        dbMsg.ConvID,
		Role:          dbMsg.Role,
		Content:       dbMsg.Content,
		InputTokens:   dbMsg.InputToken,
		OutputTokens:  dbMsg.OutputToken,
		CreatedAt:     dbMsg.CreatedAt,
		Metadata:      meta,
		IsFavorite:    dbMsg.IsFavorite,
		FavoriteLabel: dbMsg.FavoriteLabel,
	}, nil
}

func (s *PostgresStore) SetMessageFavoriteLabel(
	ctx context.Context,
	messageID string,
	label *string,
) error {
	msgUUID, err := uuid.Parse(messageID)
	if err != nil {
		return fmt.Errorf("invalid message uuid: %w", err)
	}

	result := s.db.WithContext(ctx).
		Model(&model.MessageEntity{}).
		Where("id = ?", msgUUID).
		Update("favorite_label", label)

	if result.Error != nil {
		return fmt.Errorf("failed to update favorite label: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("message not found")
	}

	return nil
}

func (s *PostgresStore) GetAllMessagesByConvID(ctx context.Context, conversationID string) ([]model.Message, error) {
	return s.GetMessagesByConvID(ctx, conversationID, 0, "asc", "")
}

func (s *PostgresStore) DeleteConversationMemorySnapshot(
	ctx context.Context,
	conversationID uuid.UUID,
	sourceType model.MemorySourceType,
) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var docIDs []uuid.UUID

		if err := tx.
			Model(&model.MemoryDocumentEntity{}).
			Where("conversation_id = ? AND source_type = ?", conversationID, sourceType).
			Pluck("id", &docIDs).Error; err != nil {
			return err
		}

		if len(docIDs) == 0 {
			return nil
		}

		if err := tx.
			Where("document_id IN ?", docIDs).
			Delete(&model.MemoryChunkEntity{}).Error; err != nil {
			return err
		}

		if err := tx.
			Where("id IN ?", docIDs).
			Delete(&model.MemoryDocumentEntity{}).Error; err != nil {
			return err
		}

		return nil
	})
}
