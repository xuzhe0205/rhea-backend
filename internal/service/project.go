package service

import (
	"context"
	"errors"
	"fmt"

	"rhea-backend/internal/model"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrProjectNotEmpty = errors.New("project still has conversations")
)

type ProjectService struct {
	store store.Store
}

func NewProjectService(s store.Store) *ProjectService {
	return &ProjectService{store: s}
}

// CreateProject creates a new project owned by the given user.
func (s *ProjectService) CreateProject(ctx context.Context, userID uuid.UUID, name, description string) (*model.Project, error) {
	p := &model.Project{
		ID:          uuid.New(),
		UserID:      userID,
		Name:        name,
		Description: description,
	}
	if err := s.store.CreateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// GetProject returns a project by ID after verifying ownership.
func (s *ProjectService) GetProject(ctx context.Context, userID, projectID uuid.UUID) (*model.Project, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.UserID != userID {
		return nil, ErrForbidden
	}
	return p, nil
}

// ListProjects returns all projects owned by a user.
func (s *ProjectService) ListProjects(ctx context.Context, userID uuid.UUID) ([]*model.Project, error) {
	return s.store.ListProjectsByUserID(ctx, userID)
}

// UpdateProject updates the name and/or description of a project after ownership check.
func (s *ProjectService) UpdateProject(ctx context.Context, userID, projectID uuid.UUID, name, description string) (*model.Project, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.UserID != userID {
		return nil, ErrForbidden
	}
	p.Name = name
	p.Description = description
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project: %w", err)
	}
	return p, nil
}

// DeleteProject deletes a project after verifying ownership and that it has no conversations.
func (s *ProjectService) DeleteProject(ctx context.Context, userID, projectID uuid.UUID) error {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p.UserID != userID {
		return ErrForbidden
	}
	count, err := s.store.CountProjectConversations(ctx, projectID)
	if err != nil {
		return fmt.Errorf("count conversations: %w", err)
	}
	if count > 0 {
		return ErrProjectNotEmpty
	}
	return s.store.DeleteProject(ctx, projectID)
}

// ListProjectConversations returns conversations belonging to a project after ownership check.
func (s *ProjectService) ListProjectConversations(ctx context.Context, userID, projectID uuid.UUID) ([]*model.Conversation, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.UserID != userID {
		return nil, ErrForbidden
	}
	return s.store.ListConversationsByProjectID(ctx, projectID)
}

// CreateProjectConversation creates a new conversation assigned to a project.
func (s *ProjectService) CreateProjectConversation(ctx context.Context, userID, projectID uuid.UUID, title string) (*model.Conversation, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.UserID != userID {
		return nil, ErrForbidden
	}
	conv := &model.Conversation{
		ID:        uuid.New(),
		UserID:    userID,
		ProjectID: &projectID,
		Title:     title,
	}
	if _, err := s.store.CreateConversation(ctx, conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}
