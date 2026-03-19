package service

import (
	"context"
	"fmt"

	"rhea-backend/internal/model"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

// AnnotationService 处理所有富文本标注相关的业务逻辑
type AnnotationService struct {
	store store.Store // 注入 PostgresStore 的接口
}

// NewAnnotationService 构造函数
func NewAnnotationService(s store.Store) *AnnotationService {
	return &AnnotationService{store: s}
}

// AnnotateMessage 是核心业务入口。它处理创建、更新以及复杂的冲突清理。
func (s *AnnotationService) AnnotateMessage(ctx context.Context, req model.Annotation) (uuid.UUID, error) {
	// 1. 基础校验：UserID 必须存在（由 Handler 从 Token 解析并注入）
	if req.UserID == uuid.Nil || req.MessageID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("missing required fields: user_id or message_id")
	}

	// 2. 冲突处理逻辑 (Quip 风格: 评论与样式互斥)
	// 规则：同一范围内，Comment 独立存在；Style 可以叠加但同类互斥。
	if req.Type == model.TypeComment {
		// 【场景：用户加评论】 清理掉该范围内所有的样式（高亮、加粗、下划线）
		styleTypes := []model.AnnotationType{model.TypeHighlight, model.TypeBold, model.TypeUnderline}
		_ = s.store.DeleteAnnotationsByRangeAndTypes(ctx, req.MessageID, req.RangeStart, req.RangeEnd, styleTypes)
	} else if isStyleType(req.Type) {
		// 【场景：用户加样式】 清理掉该范围内已有的评论
		_ = s.store.DeleteAnnotationsByRangeAndTypes(ctx, req.MessageID, req.RangeStart, req.RangeEnd, []model.AnnotationType{model.TypeComment})

		// 【场景：红改黄】 如果是同类型的样式，先清理旧的重叠部分（保证数据库不冗余）
		_ = s.store.DeleteAnnotationsByRangeAndTypes(ctx, req.MessageID, req.RangeStart, req.RangeEnd, []model.AnnotationType{req.Type})
	}

	// 3. ID 确定逻辑 (实现幂等操作)
	// 如果前端没传 ID，尝试寻找“位置+类型”完全匹配的记录，防止重复产生垃圾数据
	if req.ID == uuid.Nil {
		existing, err := s.store.GetAnnotationByFeature(ctx, req.MessageID, req.RangeStart, req.RangeEnd, req.Type)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to query existing annotation: %w", err)
		}

		if existing != nil {
			req.ID = existing.ID
		} else {
			req.ID = uuid.New()
		}
	}

	// 4. 调用存储层
	if err := s.store.SaveAnnotation(ctx, &req); err != nil {
		return uuid.Nil, fmt.Errorf("service failed to persist annotation: %w", err)
	}

	return req.ID, nil
}

// GetMessageAnnotations 获取某条消息的所有标注
func (s *AnnotationService) GetMessageAnnotations(ctx context.Context, msgID uuid.UUID, userID uuid.UUID) ([]*model.Annotation, error) {
	return s.store.ListAnnotationsByMessageID(ctx, msgID, userID)
}

// RemoveAnnotation 安全删除标注
func (s *AnnotationService) RemoveAnnotation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return s.store.DeleteAnnotation(ctx, id, userID)
}

// isStyleType 内部辅助：判断是否属于样式类
func isStyleType(t model.AnnotationType) bool {
	switch t {
	case model.TypeHighlight, model.TypeBold, model.TypeUnderline:
		return true
	default:
		return false
	}
}
