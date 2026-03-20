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

func isStyleType(t model.AnnotationType) bool {
	switch t {
	case model.TypeHighlight, model.TypeBold, model.TypeUnderline:
		return true
	default:
		return false
	}
}