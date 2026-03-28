package postgres

import (
	"context"
	"errors"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateCommentThread(ctx context.Context, thread *model.CommentThread) error {
	entity := &model.CommentThreadEntity{
		ID:                   thread.ID,
		ConvID:               thread.ConvID,
		MessageID:            thread.MessageID,
		UserID:               thread.UserID,
		RangeStart:           thread.RangeStart,
		RangeEnd:             thread.RangeEnd,
		SelectedTextSnapshot: thread.SelectedTextSnapshot,
	}

	return s.db.WithContext(ctx).Create(entity).Error
}

func (s *PostgresStore) GetCommentThreadByRange(
	ctx context.Context,
	msgID uuid.UUID,
	userID uuid.UUID,
	start, end int,
) (*model.CommentThread, error) {
	var e model.CommentThreadEntity

	err := s.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ? AND range_start = ? AND range_end = ?", msgID, userID, start, end).
		First(&e).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return s.toCommentThreadDomain(&e, nil), nil
}

func (s *PostgresStore) GetCommentThreadByID(
	ctx context.Context,
	threadID uuid.UUID,
	userID uuid.UUID,
) (*model.CommentThread, error) {
	var e model.CommentThreadEntity

	err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", threadID, userID).
		First(&e).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	comments, err := s.ListCommentsByThreadID(ctx, threadID, userID)
	if err != nil {
		return nil, err
	}

	return s.toCommentThreadDomain(&e, comments), nil
}

func (s *PostgresStore) ListCommentThreadsByMessageIDs(
	ctx context.Context,
	userID uuid.UUID,
	messageIDs []uuid.UUID,
) ([]*model.CommentThread, error) {
	if len(messageIDs) == 0 {
		return []*model.CommentThread{}, nil
	}

	var threadEntities []model.CommentThreadEntity
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND message_id IN ?", userID, messageIDs).
		Order("message_id ASC, range_start ASC, range_end ASC, created_at ASC").
		Find(&threadEntities).Error; err != nil {
		return nil, err
	}

	if len(threadEntities) == 0 {
		return []*model.CommentThread{}, nil
	}

	threadIDs := make([]uuid.UUID, 0, len(threadEntities))
	for _, t := range threadEntities {
		threadIDs = append(threadIDs, t.ID)
	}

	var commentEntities []model.CommentEntity
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND thread_id IN ?", userID, threadIDs).
		Order("created_at ASC").
		Find(&commentEntities).Error; err != nil {
		return nil, err
	}

	commentsByThreadID := make(map[uuid.UUID][]*model.Comment, len(threadIDs))
	for _, ce := range commentEntities {
		c := s.toCommentDomain(&ce)
		commentsByThreadID[ce.ThreadID] = append(commentsByThreadID[ce.ThreadID], c)
	}

	result := make([]*model.CommentThread, 0, len(threadEntities))
	for _, te := range threadEntities {
		result = append(result, s.toCommentThreadDomain(&te, commentsByThreadID[te.ID]))
	}

	return result, nil
}

func (s *PostgresStore) DeleteCommentThread(ctx context.Context, threadID uuid.UUID, userID uuid.UUID) error {
	return s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", threadID, userID).
		Delete(&model.CommentThreadEntity{}).Error
}

func (s *PostgresStore) CreateComment(ctx context.Context, comment *model.Comment) error {
	entity := &model.CommentEntity{
		ID:        comment.ID,
		ThreadID:  comment.ThreadID,
		ConvID:    comment.ConvID,
		MessageID: comment.MessageID,
		UserID:    comment.UserID,
		Content:   comment.Content,
	}

	return s.db.WithContext(ctx).Create(entity).Error
}

func (s *PostgresStore) GetCommentByID(
	ctx context.Context,
	commentID uuid.UUID,
	userID uuid.UUID,
) (*model.Comment, error) {
	var e model.CommentEntity

	err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", commentID, userID).
		First(&e).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return s.toCommentDomain(&e), nil
}

func (s *PostgresStore) ListCommentsByThreadID(
	ctx context.Context,
	threadID uuid.UUID,
	userID uuid.UUID,
) ([]*model.Comment, error) {
	var entities []model.CommentEntity

	err := s.db.WithContext(ctx).
		Where("thread_id = ? AND user_id = ?", threadID, userID).
		Order("created_at ASC").
		Find(&entities).Error
	if err != nil {
		return nil, err
	}

	results := make([]*model.Comment, len(entities))
	for i, e := range entities {
		results[i] = s.toCommentDomain(&e)
	}
	return results, nil
}

func (s *PostgresStore) DeleteComment(ctx context.Context, commentID uuid.UUID, userID uuid.UUID) error {
	return s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", commentID, userID).
		Delete(&model.CommentEntity{}).Error
}

func (s *PostgresStore) CountCommentsByThreadID(
	ctx context.Context,
	threadID uuid.UUID,
	userID uuid.UUID,
) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&model.CommentEntity{}).
		Where("thread_id = ? AND user_id = ?", threadID, userID).
		Count(&count).Error
	return count, err
}

func (s *PostgresStore) toCommentThreadDomain(e *model.CommentThreadEntity, comments []*model.Comment) *model.CommentThread {
	return &model.CommentThread{
		ID:                   e.ID,
		MessageID:            e.MessageID,
		ConvID:               e.ConvID,
		UserID:               e.UserID,
		RangeStart:           e.RangeStart,
		RangeEnd:             e.RangeEnd,
		SelectedTextSnapshot: e.SelectedTextSnapshot,
		CreatedAt:            e.CreatedAt,
		UpdatedAt:            e.UpdatedAt,
		Comments:             comments,
	}
}

func (s *PostgresStore) toCommentDomain(e *model.CommentEntity) *model.Comment {
	var deletedAt *time.Time
	if e.DeletedAt.Valid {
		deletedAt = &e.DeletedAt.Time
	}

	return &model.Comment{
		ID:        e.ID,
		ThreadID:  e.ThreadID,
		MessageID: e.MessageID,
		ConvID:    e.ConvID,
		UserID:    e.UserID,
		Content:   e.Content,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		DeletedAt: deletedAt,
	}
}
