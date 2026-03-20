package service

import (
	"context"
	"fmt"

	"rhea-backend/internal/model"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

type AnnotationService struct {
	store store.Store
}

func NewAnnotationService(s store.Store) *AnnotationService {
	return &AnnotationService{store: s}
}

func (s *AnnotationService) AnnotateMessage(ctx context.Context, req model.Annotation) (uuid.UUID, error) {
	if req.UserID == uuid.Nil || req.MessageID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("missing required fields: user_id or message_id")
	}

	if req.Type == model.TypeComment {
		styleTypes := []model.AnnotationType{model.TypeHighlight, model.TypeBold, model.TypeUnderline}
		_ = s.store.DeleteAnnotationsByRangeAndTypes(ctx, req.MessageID, req.RangeStart, req.RangeEnd, styleTypes)
	} else if isStyleType(req.Type) {
		_ = s.store.DeleteAnnotationsByRangeAndTypes(ctx, req.MessageID, req.RangeStart, req.RangeEnd, []model.AnnotationType{model.TypeComment})
		_ = s.store.DeleteAnnotationsByRangeAndTypes(ctx, req.MessageID, req.RangeStart, req.RangeEnd, []model.AnnotationType{req.Type})
	}

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

	if err := s.store.SaveAnnotation(ctx, &req); err != nil {
		return uuid.Nil, fmt.Errorf("service failed to persist annotation: %w", err)
	}

	return req.ID, nil
}

func (s *AnnotationService) GetMessageAnnotations(ctx context.Context, msgID uuid.UUID, userID uuid.UUID) ([]*model.Annotation, error) {
	return s.store.ListAnnotationsByMessageID(ctx, msgID, userID)
}

func (s *AnnotationService) GetConversationAnnotations(
	ctx context.Context,
	convID uuid.UUID,
	userID uuid.UUID,
	messageIDs []uuid.UUID,
) ([]*model.Annotation, error) {
	return s.store.ListAnnotationsByConversationAndMessageIDs(ctx, convID, userID, messageIDs)
}

func (s *AnnotationService) RemoveAnnotation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return s.store.DeleteAnnotation(ctx, id, userID)
}

func (s *AnnotationService) RemoveHighlightInRange(
	ctx context.Context,
	userID uuid.UUID,
	req model.RemoveHighlightRangeRequest,
) error {
	if userID == uuid.Nil || req.MessageID == uuid.Nil {
		return fmt.Errorf("missing required fields: user_id or message_id")
	}
	if req.RangeEnd <= req.RangeStart {
		return fmt.Errorf("invalid range")
	}

	anns, err := s.store.ListAnnotationsByMessageIDAndType(
		ctx,
		req.MessageID,
		userID,
		model.TypeHighlight,
	)
	if err != nil {
		return fmt.Errorf("failed to list existing highlights: %w", err)
	}

	type segment struct {
		start int
		end   int
	}

	var toDelete []uuid.UUID
	var toRecreate []segment

	for _, ann := range anns {
		// no overlap
		if req.RangeStart >= ann.RangeEnd || req.RangeEnd <= ann.RangeStart {
			continue
		}

		toDelete = append(toDelete, ann.ID)

		// left remainder
		if ann.RangeStart < req.RangeStart {
			leftStart := ann.RangeStart
			leftEnd := minInt(ann.RangeEnd, req.RangeStart)
			if leftEnd > leftStart {
				toRecreate = append(toRecreate, segment{start: leftStart, end: leftEnd})
			}
		}

		// right remainder
		if ann.RangeEnd > req.RangeEnd {
			rightStart := maxInt(ann.RangeStart, req.RangeEnd)
			rightEnd := ann.RangeEnd
			if rightEnd > rightStart {
				toRecreate = append(toRecreate, segment{start: rightStart, end: rightEnd})
			}
		}
	}

	if len(toDelete) == 0 {
		return nil
	}

	// 先删旧的 overlap annotations
	if err := s.store.DeleteAnnotationsByIDs(ctx, toDelete, userID); err != nil {
		return fmt.Errorf("failed to delete overlapping highlights: %w", err)
	}

	// 再插入剩余段
	for _, seg := range toRecreate {
		bg := "#FACC15"
		isNew := model.Annotation{
			ID:         uuid.New(),
			MessageID:  req.MessageID,
			ConvID:     req.ConvID,
			UserID:     userID,
			Type:       model.TypeHighlight,
			RangeStart: seg.start,
			RangeEnd:   seg.end,
			UserNote:   "",
			BgColor:    &bg,
			ExtraAttrs: map[string]interface{}{},
		}

		if err := s.store.SaveAnnotation(ctx, &isNew); err != nil {
			return fmt.Errorf("failed to recreate remaining highlight segment: %w", err)
		}
	}

	return nil
}

// Helper func

func isStyleType(t model.AnnotationType) bool {
	switch t {
	case model.TypeHighlight, model.TypeBold, model.TypeUnderline:
		return true
	default:
		return false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}