/*
Package store is:
An interface defining persistence abstraction:
- Storing conversation messages
- Retrieving recent messages
- Storing rolling summary
- Retrieving summary
- Managing Rich Text Annotations (Highlights, Comments, Styles)
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

	// --- Annotation (Rich Text) 相关 ---

	// SaveAnnotation 负责创建或更新标注
	SaveAnnotation(ctx context.Context, ann *model.Annotation) error

	// GetAnnotationByFeature 根据位置和类型寻找精确匹配的标注
	GetAnnotationByFeature(ctx context.Context, msgID uuid.UUID, start, end int, annType model.AnnotationType) (*model.Annotation, error)

	// DeleteAnnotationsByRangeAndTypes 批量删除指定范围内、特定类型的标注（用于互斥逻辑）
	DeleteAnnotationsByRangeAndTypes(ctx context.Context, msgID uuid.UUID, start, end int, types []model.AnnotationType) error

	// DeleteAnnotation 根据 ID 安全删除标注
	DeleteAnnotation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error

	// ListAnnotationsByMessageID 获取单条消息的所有标注
	ListAnnotationsByMessageID(ctx context.Context, msgID uuid.UUID, userID uuid.UUID) ([]*model.Annotation, error)
}
