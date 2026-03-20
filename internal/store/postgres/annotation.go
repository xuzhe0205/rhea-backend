package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)
// SaveAnnotation 负责创建或更新标注。
// 如果 ann.ID 已存在，则执行 Update；否则 Create。
func (s *PostgresStore) SaveAnnotation(ctx context.Context, ann *model.Annotation) error {
	if ann.ID == uuid.Nil {
		ann.ID = uuid.New()
	}

	// 1. 将 domain 字段包装进 StyleConfig JSONB
	styleMap := make(map[string]interface{})
	if ann.BgColor != nil {
		styleMap["bg_color"] = *ann.BgColor
	}
	if ann.TextColor != nil {
		styleMap["text_color"] = *ann.TextColor
	}
	if ann.IsBold != nil {
		styleMap["is_bold"] = *ann.IsBold
	}
	if ann.IsUnderline != nil {
		styleMap["is_underline"] = *ann.IsUnderline
	}

	for k, v := range ann.ExtraAttrs {
		styleMap[k] = v
	}

	styleJSON, _ := json.Marshal(styleMap)

	entity := &model.AnnotationEntity{
		ID:          ann.ID,
		MessageID:   ann.MessageID,
		ConvID:      ann.ConvID,
		UserID:      ann.UserID,
		Type:        ann.Type,
		RangeStart:  ann.RangeStart,
		RangeEnd:    ann.RangeEnd,
		UserNote:    ann.UserNote,
		StyleConfig: datatypes.JSON(styleJSON),
		UpdatedAt:   time.Now(),
	}

	// 使用 GORM 的 Save 处理 Upsert
	return s.db.WithContext(ctx).Save(entity).Error
}

// GetAnnotationByFeature 根据位置和类型寻找精确匹配的标注。
// 用于 Service 层判断：是更新现有标注还是创建新图层。
func (s *PostgresStore) GetAnnotationByFeature(
	ctx context.Context,
	msgID uuid.UUID,
	start, end int,
	annType model.AnnotationType,
) (*model.Annotation, error) {
	var e model.AnnotationEntity

	err := s.db.WithContext(ctx).
		Where("message_id = ? AND range_start = ? AND range_end = ? AND type = ?", msgID, start, end, annType).
		First(&e).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return s.toDomain(&e), nil
}

// DeleteAnnotationsByRangeAndTypes 批量删除指定范围内、特定类型的标注。
// 用于实现“同类型覆盖”（如红盖黄）或“评论样式互斥”。
func (s *PostgresStore) DeleteAnnotationsByRangeAndTypes(ctx context.Context, msgID uuid.UUID, start, end int, types []model.AnnotationType) error {
	return s.db.WithContext(ctx).
		Where("message_id = ? AND range_start = ? AND range_end = ? AND type IN ?", msgID, start, end, types).
		Delete(&model.AnnotationEntity{}).Error
}

// DeleteAnnotation 根据 ID 删除标注，并校验 UserID 确保数据安全。
func (s *PostgresStore) DeleteAnnotation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&model.AnnotationEntity{}).Error
}

// ListAnnotationsByMessageID 获取单条消息的所有标注。
// 【更新】增加了 userID 参数，并锁死查询条件以防越权查看。
func (s *PostgresStore) ListAnnotationsByMessageID(ctx context.Context, msgID uuid.UUID, userID uuid.UUID) ([]*model.Annotation, error) {
	var entities []model.AnnotationEntity
	err := s.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ?", msgID, userID). // 👈 这里必须加上 user_id
		Order("created_at ASC").
		Find(&entities).Error

	if err != nil {
		return nil, err
	}

	results := make([]*model.Annotation, len(entities))
	for i, e := range entities {
		results[i] = s.toDomain(&e)
	}
	return results, nil
}

func (s *PostgresStore) ListAnnotationsByConversationAndMessageIDs(
	ctx context.Context,
	convID uuid.UUID,
	userID uuid.UUID,
	messageIDs []uuid.UUID,
) ([]*model.Annotation, error) {
	var entities []model.AnnotationEntity

	q := s.db.WithContext(ctx).
		Where("conv_id = ? AND user_id = ?", convID, userID).
		Order("created_at ASC")

	if len(messageIDs) > 0 {
		q = q.Where("message_id IN ?", messageIDs)
	}

	err := q.Find(&entities).Error
	if err != nil {
		return nil, err
	}

	results := make([]*model.Annotation, len(entities))
	for i, e := range entities {
		results[i] = s.toDomain(&e)
	}
	return results, nil
}

func (s *PostgresStore) ListAnnotationsByMessageIDAndType(
	ctx context.Context,
	msgID uuid.UUID,
	userID uuid.UUID,
	annType model.AnnotationType,
) ([]*model.Annotation, error) {
	var entities []model.AnnotationEntity
	err := s.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ? AND type = ?", msgID, userID, annType).
		Order("created_at ASC").
		Find(&entities).Error
	if err != nil {
		return nil, err
	}

	results := make([]*model.Annotation, len(entities))
	for i, e := range entities {
		results[i] = s.toDomain(&e)
	}
	return results, nil
}

func (s *PostgresStore) DeleteAnnotationsByIDs(
	ctx context.Context,
	ids []uuid.UUID,
	userID uuid.UUID,
) error {
	if len(ids) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).
		Where("id IN ? AND user_id = ?", ids, userID).
		Delete(&model.AnnotationEntity{}).Error
}

// --- 内部转换逻辑 ---

func (s *PostgresStore) toDomain(e *model.AnnotationEntity) *model.Annotation {
	var styleMap map[string]interface{}
	_ = json.Unmarshal(e.StyleConfig, &styleMap)

	return &model.Annotation{
		ID:         e.ID,
		MessageID:  e.MessageID,
		ConvID:     e.ConvID,
		UserID:     e.UserID,
		Type:       e.Type,
		RangeStart: e.RangeStart,
		RangeEnd:   e.RangeEnd,
		UserNote:   e.UserNote,
		// 调用 internal/store/postgres/util.go 中的工具函数
		BgColor:     getStringPtr(styleMap, "bg_color"),
		TextColor:   getStringPtr(styleMap, "text_color"),
		IsBold:      getBoolPtr(styleMap, "is_bold"),
		IsUnderline: getBoolPtr(styleMap, "is_underline"),
		ExtraAttrs:  styleMap,
	}
}
