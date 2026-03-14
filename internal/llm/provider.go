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

// Usage 结构体建议保持，它是我们账单的核心
type Usage struct {
	InputTokens  int
	OutputTokens int
	ModelName    string
}

// ChatResponse 包装了非流式调用的结果
type ChatResponse struct {
	Content string
	Usage   Usage
}

type Provider interface {
	Name() ProviderName
	ModelName() string

	// 🚀 修改：Chat 不再只返回 string，而是返回完整的 ChatResponse
	Chat(ctx context.Context, messages []model.Message) (ChatResponse, error)

	// 🚀 修改：emit 回调新增第二个参数 *Usage
	// 当传输进行中，usage 为 nil；当最后一条 chunk 到达且包含统计时，usage 为非空
	Stream(ctx context.Context, messages []model.Message, emit func(delta string, usage *Usage) error) error
}
