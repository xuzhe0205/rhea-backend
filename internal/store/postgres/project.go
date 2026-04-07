package postgres

import (
	"context"
	"fmt"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateProject(ctx context.Context, project *model.Project) error {
	if project.ID == uuid.Nil {
		project.ID = uuid.New()
	}
	now := time.Now()
	project.CreatedAt = now
	project.UpdatedAt = now

	entity := &model.ProjectEntity{
		ID:          project.ID,
		UserID:      project.UserID,
		Name:        project.Name,
		Description: project.Description,
		Summary:     project.Summary,
	}
	if err := s.db.WithContext(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	project.CreatedAt = entity.CreatedAt
	project.UpdatedAt = entity.UpdatedAt
	return nil
}

func (s *PostgresStore) GetProject(ctx context.Context, id uuid.UUID) (*model.Project, error) {
	var entity model.ProjectEntity
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&entity).Error; err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	return projectFromEntity(&entity), nil
}

func (s *PostgresStore) ListProjectsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Project, error) {
	var entities []model.ProjectEntity
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	results := make([]*model.Project, len(entities))
	for i, e := range entities {
		results[i] = projectFromEntity(&e)
	}
	return results, nil
}

func (s *PostgresStore) UpdateProject(ctx context.Context, project *model.Project) error {
	updates := map[string]interface{}{
		"name":        project.Name,
		"description": project.Description,
		"summary":     project.Summary,
		"updated_at":  time.Now(),
	}
	result := s.db.WithContext(ctx).
		Model(&model.ProjectEntity{}).
		Where("id = ?", project.ID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteProject(ctx context.Context, id uuid.UUID) error {
	result := s.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.ProjectEntity{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *PostgresStore) CountProjectConversations(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&model.ConversationEntity{}).
		Where("project_id = ?", projectID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count project conversations: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) ListConversationsByProjectID(ctx context.Context, projectID uuid.UUID) ([]*model.Conversation, error) {
	var entities []model.ConversationEntity
	if err := s.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("updated_at DESC").
		Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to list project conversations: %w", err)
	}

	results := make([]*model.Conversation, len(entities))
	for i, e := range entities {
		results[i] = &model.Conversation{
			ID:               e.ID,
			UserID:           e.UserID,
			ProjectID:        e.ProjectID,
			Title:            e.Title,
			Summary:          e.Summary,
			LastMsgID:        e.LastMsgID,
			IsPinned:         e.IsPinned,
			PinnedAt:         e.PinnedAt,
			CumulativeTokens: e.TokenSum,
			CreatedAt:        e.CreatedAt,
			UpdatedAt:        e.UpdatedAt,
		}
	}
	return results, nil
}

func (s *PostgresStore) AssignConversationToProject(ctx context.Context, conversationID uuid.UUID, projectID uuid.UUID) error {
	result := s.db.WithContext(ctx).
		Model(&model.ConversationEntity{}).
		Where("id = ?", conversationID).
		Updates(map[string]interface{}{
			"project_id": projectID,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to assign conversation to project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// projectFromEntity maps a ProjectEntity to the Project domain model.
func projectFromEntity(e *model.ProjectEntity) *model.Project {
	return &model.Project{
		ID:          e.ID,
		UserID:      e.UserID,
		Name:        e.Name,
		Description: e.Description,
		Summary:     e.Summary,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}
