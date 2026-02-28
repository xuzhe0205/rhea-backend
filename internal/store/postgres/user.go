package postgres

import (
	"context"
	"encoding/json"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
)

// GetUserByEmail 根据邮箱查找用户
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var entity model.UserEntity
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&entity).Error; err != nil {
		return nil, err
	}

	return &model.User{
		ID:           entity.ID,
		Email:        entity.Email,
		PasswordHash: entity.PasswordHash,
		UserName:     entity.UserName,
	}, nil
}

// CreateUser 创建新用户
func (s *PostgresStore) CreateUser(ctx context.Context, user *model.User) (*model.User, error) {
	entity := &model.UserEntity{
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		UserName:     user.UserName,
	}

	if err := s.db.WithContext(ctx).Create(entity).Error; err != nil {
		return nil, err
	}

	user.ID = entity.ID
	return user, nil
}

// GetUserByID 根据 ID 查找用户（含 Metadata 解析）
func (s *PostgresStore) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var entity model.UserEntity
	if err := s.db.WithContext(ctx).First(&entity, "id = ?", id).Error; err != nil {
		return nil, err
	}

	var meta map[string]interface{}
	if len(entity.Metadata) > 0 {
		if err := json.Unmarshal(entity.Metadata, &meta); err != nil {
			meta = make(map[string]interface{})
		}
	}

	return &model.User{
		ID:           entity.ID,
		Email:        entity.Email,
		PasswordHash: entity.PasswordHash,
		UserName:     entity.UserName,
		Metadata:     meta,
		CreatedAt:    entity.CreatedAt,
	}, nil
}
