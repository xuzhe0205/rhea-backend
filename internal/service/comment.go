package service

import (
	"context"
	"fmt"
	"strings"

	"rhea-backend/internal/model"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

type CommentService struct {
	store store.Store
}

func NewCommentService(s store.Store) *CommentService {
	return &CommentService{store: s}
}

func (s *CommentService) GetCommentThread(
	ctx context.Context,
	msgID uuid.UUID,
	userID uuid.UUID,
	start, end int,
) (*model.CommentThread, error) {
	if msgID == uuid.Nil || userID == uuid.Nil {
		return nil, fmt.Errorf("missing required fields: message_id or user_id")
	}
	if end <= start {
		return nil, fmt.Errorf("invalid range")
	}

	thread, err := s.store.GetCommentThreadByRange(ctx, msgID, userID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment thread by range: %w", err)
	}
	if thread == nil {
		return nil, nil
	}

	comments, err := s.store.ListCommentsByThreadID(ctx, thread.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list comments by thread id: %w", err)
	}

	thread.Comments = comments
	return thread, nil
}

func (s *CommentService) GetComment(
	ctx context.Context,
	commentID uuid.UUID,
	userID uuid.UUID,
) (*model.Comment, error) {
	if commentID == uuid.Nil || userID == uuid.Nil {
		return nil, fmt.Errorf("missing required fields: comment_id or user_id")
	}

	comment, err := s.store.GetCommentByID(ctx, commentID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment by id: %w", err)
	}

	return comment, nil
}

func (s *CommentService) AddComment(
	ctx context.Context,
	userID uuid.UUID,
	req model.AddCommentRequest,
) (*model.CommentThread, *model.Comment, error) {
	if userID == uuid.Nil || req.MessageID == uuid.Nil || req.ConvID == uuid.Nil {
		return nil, nil, fmt.Errorf("missing required fields: user_id, message_id or conv_id")
	}
	if req.RangeEnd <= req.RangeStart {
		return nil, nil, fmt.Errorf("invalid range")
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, nil, fmt.Errorf("comment content cannot be empty")
	}
	if len([]rune(content)) > 500 {
		return nil, nil, fmt.Errorf("comment content exceeds 500 characters")
	}

	thread, err := s.store.GetCommentThreadByRange(ctx, req.MessageID, userID, req.RangeStart, req.RangeEnd)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query existing comment thread: %w", err)
	}

	if thread == nil {
		thread = &model.CommentThread{
			ID:                   uuid.New(),
			MessageID:            req.MessageID,
			ConvID:               req.ConvID,
			UserID:               userID,
			RangeStart:           req.RangeStart,
			RangeEnd:             req.RangeEnd,
			SelectedTextSnapshot: req.SelectedTextSnapshot,
		}

		if err := s.store.CreateCommentThread(ctx, thread); err != nil {
			return nil, nil, fmt.Errorf("failed to create comment thread: %w", err)
		}
	}

	comment := &model.Comment{
		ID:        uuid.New(),
		ThreadID:  thread.ID,
		MessageID: req.MessageID,
		ConvID:    req.ConvID,
		UserID:    userID,
		Content:   content,
	}

	if err := s.store.CreateComment(ctx, comment); err != nil {
		return nil, nil, fmt.Errorf("failed to create comment: %w", err)
	}

	threadWithComments, err := s.store.GetCommentThreadByID(ctx, thread.ID, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reload comment thread: %w", err)
	}

	return threadWithComments, comment, nil
}

func (s *CommentService) DeleteComment(
	ctx context.Context,
	commentID uuid.UUID,
	userID uuid.UUID,
) error {
	if commentID == uuid.Nil || userID == uuid.Nil {
		return fmt.Errorf("missing required fields: comment_id or user_id")
	}

	comment, err := s.store.GetCommentByID(ctx, commentID, userID)
	if err != nil {
		return fmt.Errorf("failed to get comment before delete: %w", err)
	}
	if comment == nil {
		return fmt.Errorf("comment not found")
	}

	if err := s.store.DeleteComment(ctx, commentID, userID); err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	count, err := s.store.CountCommentsByThreadID(ctx, comment.ThreadID, userID)
	if err != nil {
		return fmt.Errorf("failed to count remaining comments in thread: %w", err)
	}

	if count == 0 {
		if err := s.store.DeleteCommentThread(ctx, comment.ThreadID, userID); err != nil {
			return fmt.Errorf("failed to delete empty comment thread: %w", err)
		}
	}

	return nil
}

func (s *CommentService) GetCommentThreadsByMessageIDs(
	ctx context.Context,
	userID uuid.UUID,
	messageIDs []uuid.UUID,
) ([]*model.CommentThread, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("missing required field: user_id")
	}
	if len(messageIDs) == 0 {
		return []*model.CommentThread{}, nil
	}

	threads, err := s.store.ListCommentThreadsByMessageIDs(ctx, userID, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list comment threads by message ids: %w", err)
	}

	return threads, nil
}
