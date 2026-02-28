package auth

import (
	"context"
	"errors"
	"fmt"
	"rhea-backend/internal/model"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

type Service struct {
	Store store.Store
}

func NewService(st store.Store) *Service {
	return &Service{Store: st}
}

// Register 处理用户注册
func (s *Service) Register(ctx context.Context, email, password string) (*model.User, error) {
	// 第一关：格式检查 (纯内存操作，极快)
	if err := ValidateCredentials(email, password); err != nil {
		return nil, err
	}

	// 第二关：唯一性检查 (需要跨网络查 DB，慢)
	existing, _ := s.Store.GetUserByEmail(ctx, email)
	if existing != nil {
		return nil, errors.New("email already registered")
	}

	// 3. 加密密码
	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// 4. 构造并存入数据库
	user := &model.User{
		Email:        email,
		PasswordHash: hash,
		UserName:     email, // 默认用户名暂设为邮箱
	}

	return s.Store.CreateUser(ctx, user)
}

// Login 处理用户登录并返回 JWT
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	user, err := s.Store.GetUserByEmail(ctx, email)
	if err != nil {
		return "", errors.New("invalid email or password")
	}

	// 验证密码
	if !CheckPasswordHash(password, user.PasswordHash) {
		return "", errors.New("invalid email or password")
	}

	// 签发 Token
	return GenerateToken(user.ID)
}

// Get current user from JWT
func (s *Service) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return s.Store.GetUserByID(ctx, id)
}
