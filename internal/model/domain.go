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

type Message struct {
	ID           uuid.UUID              `json:"id"`
	Role         Role                   `json:"role"`
	Content      string                 `json:"content"`
	InputTokens  int                    `json:"input_tokens"`  // 对应 prompt_token_count
	OutputTokens int                    `json:"output_tokens"` // 对应 candidates_token_count
	CreatedAt    time.Time              `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type Conversation struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	Title            string     `json:"title"`
	LastMsgID        *uuid.UUID `json:"last_msg_id"`
	Summary          string     `json:"summary"`
	CumulativeTokens int        `json:"cumulative_tokens"` // 累计消耗，用于触发 100w 警告
	LastSummaryAt    time.Time  `json:"last_summary_at"`   // 记录上次总结的时间或消息位置
}

type User struct {
	ID           uuid.UUID              `json:"id"`
	Email        string                 `json:"email"`
	PasswordHash string                 `json:"-"`
	UserName     string                 `json:"user_name"`
	CreatedAt    time.Time              `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}
