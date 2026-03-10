package llm

import (
	"context"

	"rhea-backend/internal/model"
)

type ProviderName string

const (
	ProviderClaude ProviderName = "claude"
	ProviderGemini ProviderName = "gemini"
	ProviderOpenAI ProviderName = "openai"
)

type Provider interface {
	Name() ProviderName
	ModelName() string // 🚀 新增：返回具体的模型型号，如 "gemini-1.5-pro"
	Chat(ctx context.Context, messages []model.Message) (string, error)
	Stream(ctx context.Context, messages []model.Message, emit func(delta string) error) error
}
