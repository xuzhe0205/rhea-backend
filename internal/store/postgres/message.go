package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
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

// func (s *PostgresStore) GetRecentMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error) {
// 	var dbMsgs []model.MessageEntity

// 	query := s.db.WithContext(ctx).
// 		Where("conv_id = ?", conversationID).
// 		Order("created_at desc")

// 	if limit > 0 {
// 		query = query.Limit(limit)
// 	}

// 	if err := query.Find(&dbMsgs).Error; err != nil {
// 		return nil, err
// 	}

// 	var msgs []model.Message
// 	for i := len(dbMsgs) - 1; i >= 0; i-- {
// 		msgs = append(msgs, model.Message{
// 			Role:    dbMsgs[i].Role,
// 			Content: dbMsgs[i].Content,
// 		})
// 	}
// 	return msgs, nil
// }

func (s *PostgresStore) GetMessagesByConvID(ctx context.Context, conversationID string, limit int, order string, beforeID string) ([]model.Message, error) {
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, fmt.Errorf("invalid conversation uuid: %w", err)
	}

	query := s.db.WithContext(ctx).Where("conv_id = ?", convUUID)

	// 1. 处理游标逻辑 (Cursor Pagination)
	if beforeID != "" {
		var beforeMsg model.MessageEntity
		// 先找到参考消息的创建时间
		if err := s.db.Select("created_at").First(&beforeMsg, "id = ?", beforeID).Error; err != nil {
			return nil, fmt.Errorf("before_id not found: %w", err)
		}
		// 核心：查询比这条消息更早的消息
		query = query.Where("created_at < ?", beforeMsg.CreatedAt)
	}

	// 2. 数据库查询强制使用降序 (DESC)
	// 只有降序排列加上 Limit，才能拿到“紧挨着游标”的较旧消息
	query = query.Order("created_at desc")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var dbMsgs []model.MessageEntity
	if err := query.Find(&dbMsgs).Error; err != nil {
		return nil, err
	}

	// 3. 搬运到 Domain Model
	msgs := make([]model.Message, len(dbMsgs))
	for i, m := range dbMsgs {
		var meta map[string]interface{}
		if len(m.Metadata) > 0 {
			_ = json.Unmarshal(m.Metadata, &meta)
		}

		msgs[i] = model.Message{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
			Metadata:  meta,
		}
	}

	// 4. 关键：在内存中反转顺序 (Reverse)
	// 如果前端需要的是展示用的升序 (asc)，我们就把 [新...旧] 反转为 [旧...新]
	if order == "asc" {
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
	}

	return msgs, nil
}
