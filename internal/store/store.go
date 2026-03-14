/*
Package store is:
An interface defining persistence abstraction:
- Storing conversation messages
- Retrieving recent messages
- Storing rolling summary
- Retrieving summary
*/
package store

import (
	"context"

	"github.com/google/uuid"

	"rhea-backend/internal/model"
)

type Store interface {
	// Message 相关
	AppendMessage(ctx context.Context, conversationID string, parentID *string, msg model.Message, metadata map[string]interface{}) (string, error)
	// GetRecentMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error)
	GetMessagesByConvID(ctx context.Context, conversationID string, limit int, order string, beforeID string) ([]model.Message, error)

	// Conversation 相关
	GetConversation(ctx context.Context, id string) (*model.Conversation, error)
	CreateConversation(ctx context.Context, conv *model.Conversation) (string, error)
	UpdateConversationStatus(ctx context.Context, convID string, newLastMsgID string, expectedOldMsgID *string, tokenDelta int) (int, error)
	UpdateConversationTitle(ctx context.Context, convID string, title string) error
	ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error)
	IncrementConversationTokenUsage(ctx context.Context, convID string, delta int) error

	// Summary 相关
	GetSummary(ctx context.Context, conversationID string) (string, error)
	SetSummary(ctx context.Context, conversationID string, summary string) error

	// User 相关
	CreateUser(ctx context.Context, user *model.User) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}
