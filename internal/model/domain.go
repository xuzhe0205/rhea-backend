// Package model includes what each entity being stored in table should look like in API request
package model

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ImageAttachment carries image bytes for a single message turn.
// It is never persisted — only used in-memory during LLM inference.
type ImageAttachment struct {
	MIMEType string
	Data     []byte
}

type Message struct {
	ID            uuid.UUID              `json:"id"`
	ConvID        uuid.UUID              `json:"conv_id"`
	Role          Role                   `json:"role"`
	Content       string                 `json:"content"`
	InputTokens   int                    `json:"input_tokens"`
	OutputTokens  int                    `json:"output_tokens"`
	CreatedAt     time.Time             `json:"created_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	IsFavorite    bool                   `json:"is_favorite"`
	FavoriteLabel *string                `json:"favorite_label,omitempty"`
	Images        []ImageAttachment      `json:"-"` // in-memory only
}

type Conversation struct {
	ID                    uuid.UUID  `json:"id"`
	UserID                uuid.UUID  `json:"user_id"`
	ProjectID             *uuid.UUID `json:"project_id,omitempty"`
	Title                 string     `json:"title"`
	LastMsgID             *uuid.UUID `json:"last_msg_id"`
	Summary               string     `json:"summary"`
	IsPinned              bool       `json:"is_pinned"`
	PinnedAt              *time.Time `json:"pinned_at,omitempty"`
	CumulativeTokens      int        `json:"cumulative_tokens"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	SummaryUpdatedAt      *time.Time `json:"summary_updated_at,omitempty"`
	MemoryCheckpointMsgID *uuid.UUID `json:"memory_checkpoint_msg_id,omitempty"`
}

type User struct {
	ID           uuid.UUID              `json:"id"`
	Email        string                 `json:"email"`
	PasswordHash string                 `json:"-"`
	UserName     string                 `json:"user_name"`
	CreatedAt    time.Time              `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// Annotation 代表用户对消息的标注、笔记或收藏请求
type Annotation struct {
	ID        uuid.UUID `json:"id"`
	MessageID uuid.UUID `json:"message_id"`
	ConvID    uuid.UUID `json:"conv_id"`
	UserID    uuid.UUID `json:"user_id"`

	// 使用 model 中定义的枚举：highlight, comment, favorite, bold, underline
	Type AnnotationType `json:"type"`

	// 选中的字符范围
	RangeStart int `json:"range_start"`
	RangeEnd   int `json:"range_end"`

	// 用户输入的主观笔记/评论
	UserNote string `json:"user_note"`

	// --- 样式字段 (使用指针以支持 Nullable/Optional) ---

	// 背景颜色 (如 "#FFD700")，对应 Highlight
	BgColor *string `json:"bg_color"`

	// 文字颜色 (如 "#FF0000")
	TextColor *string `json:"text_color"`

	// 是否加粗
	IsBold *bool `json:"is_bold"`

	// 是否下划线
	IsUnderline *bool `json:"is_underline"`

	// 扩展字段：如果以后有特殊的前端需求，可以先塞在这里
	ExtraAttrs map[string]interface{} `json:"extra_attrs"`
}

type RemoveHighlightRangeRequest struct {
	MessageID  uuid.UUID `json:"message_id"`
	ConvID     uuid.UUID `json:"conv_id"`
	RangeStart int       `json:"range_start"`
	RangeEnd   int       `json:"range_end"`
}

type FavoriteMessageRow struct {
	ID            uuid.UUID  `json:"id"`
	ConvID        uuid.UUID  `json:"conversationId"`
	ProjectID     *uuid.UUID `json:"project_id,omitempty"`
	Role          Role       `json:"role"`
	Content       string     `json:"content"`
	CreatedAt     time.Time  `json:"createdAt"`
	FavoritedAt   *time.Time `json:"favoritedAt"`
	FavoriteLabel *string    `json:"favorite_label,omitempty"`
}

type Project struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Summary     string    `json:"summary"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CommentThread struct {
	ID                   uuid.UUID  `json:"id"`
	MessageID            uuid.UUID  `json:"message_id"`
	ConvID               uuid.UUID  `json:"conv_id"`
	UserID               uuid.UUID  `json:"user_id"`
	RangeStart           int        `json:"range_start"`
	RangeEnd             int        `json:"range_end"`
	SelectedTextSnapshot string     `json:"selected_text_snapshot"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Comments             []*Comment `json:"comments,omitempty"`
}

type Comment struct {
	ID        uuid.UUID  `json:"id"`
	ThreadID  uuid.UUID  `json:"thread_id"`
	MessageID uuid.UUID  `json:"message_id"`
	ConvID    uuid.UUID  `json:"conv_id"`
	UserID    uuid.UUID  `json:"user_id"`
	Content   string     `json:"content"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type GetCommentThreadRequest struct {
	MessageID  uuid.UUID `json:"message_id"`
	RangeStart int       `json:"range_start"`
	RangeEnd   int       `json:"range_end"`
}

type AddCommentRequest struct {
	MessageID            uuid.UUID `json:"message_id"`
	ConvID               uuid.UUID `json:"conv_id"`
	RangeStart           int       `json:"range_start"`
	RangeEnd             int       `json:"range_end"`
	SelectedTextSnapshot string    `json:"selected_text_snapshot"`
	Content              string    `json:"content"`
}

type ListCommentThreadsByMessageIDsRequest struct {
	MessageIDs []uuid.UUID `json:"message_ids"`
}
