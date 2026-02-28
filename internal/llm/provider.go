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
	Chat(ctx context.Context, messages []model.Message) (string, error)

	// Streaming: provider calls emit(delta) multiple times.
	// emit should write a chunk to the client (or buffer in tests).
	Stream(ctx context.Context, messages []model.Message, emit func(delta string) error) error
}
