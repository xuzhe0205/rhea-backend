package llm

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"rhea-backend/internal/model"
)

type GeminiProvider struct {
	Model       string
	Client      *genai.Client
	Temperature float32
}

func (p *GeminiProvider) Name() ProviderName { return ProviderGemini }

func NewGeminiProvider(ctx context.Context, apiKey string, model string, temp float32) (*GeminiProvider, error) {
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiProvider{
		Model:       model,
		Client:      c,
		Temperature: temp,
	}, nil
}

// Chat implements Provider.Chat (non-streaming).
func (p *GeminiProvider) Chat(ctx context.Context, msgs []model.Message) (ChatResponse, error) {
	// 统一使用 extractSystemPrompt
	systemText, contents := extractSystemPrompt(msgs)

	t := p.Temperature
	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemText}},
		},
		Temperature: &t,
	}

	// 针对 Lite/Pro 的差异化配置
	isLite := strings.Contains(strings.ToLower(p.Model), "lite")
	if isLite {
		cfg.MaxOutputTokens = 10
	} else {
		cfg.Tools = []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}}
	}

	resp, err := p.Client.Models.GenerateContent(ctx, p.Model, contents, cfg)
	if err != nil {
		return ChatResponse{}, err
	}
	// SDK provides helpers, but this is robust enough for v1:
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return ChatResponse{}, fmt.Errorf("gemini: empty response")
	}
	// Usually first text part:
	if t := resp.Candidates[0].Content.Parts[0].Text; t != "" {
		return ChatResponse{
			Content: t,
			Usage: Usage{
				InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
				OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
				ModelName:    p.Model,
			},
		}, nil
	}
	return ChatResponse{}, fmt.Errorf("gemini: unexpected empty text part")
}

// Stream implements Provider.ChatStream (streaming).
func (p *GeminiProvider) Stream(ctx context.Context, msgs []model.Message, emit func(delta string, usage *Usage) error) error {
	systemText, contents := extractSystemPrompt(msgs)

	// 同样处理 Temperature 指针
	t := p.Temperature

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemText}},
		},
		Temperature: &t, // 👈 应用新增字段
		Tools:       []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}},
	}

	// Lite 模型通常不用于 Stream，但为了代码健壮性，我们可以加个判断
	if strings.Contains(strings.ToLower(p.Model), "lite") {
		cfg.Tools = nil
	}

	iter := p.Client.Models.GenerateContentStream(ctx, p.Model, contents, cfg)
	for resp, err := range iter {
		if err != nil {
			return err
		}
		var delta string
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil && len(resp.Candidates[0].Content.Parts) > 0 {
			delta = resp.Candidates[0].Content.Parts[0].Text
		}
		// --- 核心修改：提取并传递 Usage ---
		var u *Usage
		if resp.UsageMetadata != nil {
			u = &Usage{
				InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
				OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
				ModelName:    p.Model,
			}
		}

		// 即使 delta 为空，如果 u 不为空（最后一次 metadata），也得发出去
		if delta != "" || u != nil {
			if err := emit(delta, u); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- helpers ---

func extractSystemPrompt(msgs []model.Message) (string, []*genai.Content) {
	var systemText string
	contents := make([]*genai.Content, 0)

	for _, m := range msgs {
		if m.Role == model.RoleSystem {
			systemText = m.Content // 拿到那个“2026年”的指令
			continue
		}

		role := genai.RoleUser
		if m.Role == model.RoleAssistant {
			role = genai.RoleModel
		}

		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: m.Content}},
		})
	}
	return systemText, contents
}

func toGenAIContents(msgs []model.Message) []*genai.Content {
	out := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := genai.RoleUser
		// Gemini SDK uses role "model" for assistant. :contentReference[oaicite:5]{index=5}
		if m.Role == model.RoleAssistant {
			role = genai.RoleModel
		}
		// You can either keep system prompt as a "user" message (simple v1),
		// or later use "system instruction" config.
		if m.Role == model.RoleSystem {
			role = genai.RoleUser
		}

		out = append(out, &genai.Content{
			Role: role,
			Parts: []*genai.Part{
				{Text: m.Content},
			},
		})
	}
	return out
}

func (p *GeminiProvider) ModelName() string {
	return p.Model
}
