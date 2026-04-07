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
	"rhea-backend/internal/rag"
)

type MemoryChunkSearchResult struct {
	Chunk        model.MemoryChunkEntity
	VectorScore  float64
	KeywordScore float64
}

type Store interface {
	// Message 相关
	AppendMessage(ctx context.Context, conversationID string, parentID *string, msg model.Message, metadata map[string]interface{}) (string, error)
	GetMessagesByConvID(ctx context.Context, conversationID string, limit int, order string, beforeID string) ([]model.Message, error)
	GetMessagesForFavoriteJump(ctx context.Context, conversationID string, messageID string, olderBuffer int) ([]model.Message, error)
	SetMessageFavorite(ctx context.Context, messageID string, isFavorite bool) error
	ListFavoriteMessages(ctx context.Context, userID string, limit int, offset int) ([]model.FavoriteMessageRow, error)
	GetMessageByID(ctx context.Context, messageID string) (*model.Message, error)
	SetMessageFavoriteLabel(ctx context.Context, messageID string, label *string) error

	// Conversation 相关
	GetConversation(ctx context.Context, id string) (*model.Conversation, error)
	CreateConversation(ctx context.Context, conv *model.Conversation) (string, error)
	UpdateConversationStatus(ctx context.Context, convID string, newLastMsgID string, expectedOldMsgID *string, tokenDelta int) (int, error)
	UpdateConversationTitle(ctx context.Context, convID string, title string) error
	ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error)
	IncrementConversationTokenUsage(ctx context.Context, convID string, delta int) error
	ListPinnedConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error)
	SetConversationPinned(ctx context.Context, convID string, isPinned bool) error

	// Project 相关
	CreateProject(ctx context.Context, project *model.Project) error
	GetProject(ctx context.Context, id uuid.UUID) (*model.Project, error)
	ListProjectsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Project, error)
	UpdateProject(ctx context.Context, project *model.Project) error
	DeleteProject(ctx context.Context, id uuid.UUID) error
	CountProjectConversations(ctx context.Context, projectID uuid.UUID) (int64, error)
	ListConversationsByProjectID(ctx context.Context, projectID uuid.UUID) ([]*model.Conversation, error)
	AssignConversationToProject(ctx context.Context, conversationID uuid.UUID, projectID uuid.UUID) error

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

	ListAnnotationsByConversationAndMessageIDs(ctx context.Context, convID uuid.UUID, userID uuid.UUID, messageIDs []uuid.UUID) ([]*model.Annotation, error)

	ListAnnotationsByMessageIDAndType(ctx context.Context, msgID uuid.UUID, userID uuid.UUID, annType model.AnnotationType) ([]*model.Annotation, error)

	DeleteAnnotationsByIDs(ctx context.Context, ids []uuid.UUID, userID uuid.UUID) error

	// --- Comment Thread / Comment 相关 ---

	CreateCommentThread(ctx context.Context, thread *model.CommentThread) error
	GetCommentThreadByRange(ctx context.Context, msgID uuid.UUID, userID uuid.UUID, start, end int) (*model.CommentThread, error)
	GetCommentThreadByID(ctx context.Context, threadID uuid.UUID, userID uuid.UUID) (*model.CommentThread, error)
	DeleteCommentThread(ctx context.Context, threadID uuid.UUID, userID uuid.UUID) error

	CreateComment(ctx context.Context, comment *model.Comment) error
	GetCommentByID(ctx context.Context, commentID uuid.UUID, userID uuid.UUID) (*model.Comment, error)
	ListCommentsByThreadID(ctx context.Context, threadID uuid.UUID, userID uuid.UUID) ([]*model.Comment, error)
	DeleteComment(ctx context.Context, commentID uuid.UUID, userID uuid.UUID) error
	CountCommentsByThreadID(ctx context.Context, threadID uuid.UUID, userID uuid.UUID) (int64, error)
	ListCommentThreadsByMessageIDs(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID) ([]*model.CommentThread, error)

	CreateMemoryDocument(ctx context.Context, doc *model.MemoryDocumentEntity) error
	BulkCreateMemoryChunks(ctx context.Context, chunks []model.MemoryChunkEntity) error
	BulkCreateMemoryEmbeddings(ctx context.Context, embeddings []model.MemoryEmbeddingEntity) error

	// RAG
	// ===== Memory read path =====
	VectorSearchMemoryChunks(
		ctx context.Context,
		userID uuid.UUID,
		conversationID uuid.UUID,
		projectID *uuid.UUID,
		scope rag.Scope,
		queryEmbedding []float32,
		limit int,
	) ([]MemoryChunkSearchResult, error)

	KeywordSearchMemoryChunks(
		ctx context.Context,
		userID uuid.UUID,
		conversationID uuid.UUID,
		projectID *uuid.UUID,
		scope rag.Scope,
		query string,
		ftsConfig string,
		limit int,
	) ([]MemoryChunkSearchResult, error)

	MarkMemoryDocumentIndexed(
		ctx context.Context,
		documentID uuid.UUID,
	) error

	MarkMemoryDocumentFailed(
		ctx context.Context,
		documentID uuid.UUID,
		errMsg string,
	) error

	DeactivateActiveMemoryDocuments(
		ctx context.Context,
		conversationID uuid.UUID,
		sourceType model.MemorySourceType,
		excludeDocumentID uuid.UUID,
	) error

	// ===== Memory checkpoint =====
	UpdateConversationMemoryCheckpoint(
		ctx context.Context,
		conversationID uuid.UUID,
		checkpointMsgID uuid.UUID,
	) error

	DeleteConversationMemorySnapshot(
		ctx context.Context,
		conversationID uuid.UUID,
		sourceType model.MemorySourceType,
	) error

	GetAllMessagesByConvID(
		ctx context.Context,
		conversationID string,
	) ([]model.Message, error)
}
