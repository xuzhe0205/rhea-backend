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
	ID        uuid.UUID              `json:"id"` // 👈 新增：消息唯一标识
	Role      Role                   `json:"role"`
	Content   string                 `json:"content"`
	CreatedAt time.Time              `json:"created_at"` // 👈 新增：时间戳
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type Conversation struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	Title     string     `json:"title"`
	LastMsgID *uuid.UUID `json:"last_msg_id"`
	Summary   string     `json:"summary"`
}

type User struct {
	ID           uuid.UUID              `json:"id"`
	Email        string                 `json:"email"`
	PasswordHash string                 `json:"-"`
	UserName     string                 `json:"user_name"`
	CreatedAt    time.Time              `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}
