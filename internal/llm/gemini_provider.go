package llm

import (
	"context"
	"fmt"

	"google.golang.org/genai"

	"rhea-backend/internal/model"
)

type GeminiProvider struct {
	Model  string
	Client *genai.Client
}

func (p *GeminiProvider) Name() ProviderName { return ProviderGemini }

func NewGeminiProvider(ctx context.Context, apiKey string, model string) (*GeminiProvider, error) {
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	if model == "" {
		// Free tier
		model = "gemini-2.5-flash"
	}
	return &GeminiProvider{Model: model, Client: c}, nil
}

// Chat implements Provider.Chat (non-streaming).
func (p *GeminiProvider) Chat(ctx context.Context, msgs []model.Message) (string, error) {
	contents := toGenAIContents(msgs)

	// 创建配置并添加工具
	cfg := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}}, // 新 SDK 开启联网的方式
		},
	}

	resp, err := p.Client.Models.GenerateContent(ctx, p.Model, contents, cfg) // 传入 cfg
	if err != nil {
		return "", err
	}
	// SDK provides helpers, but this is robust enough for v1:
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}
	// Usually first text part:
	if t := resp.Candidates[0].Content.Parts[0].Text; t != "" {
		return t, nil
	}
	return resp.Text(), nil
}

// Stream implements Provider.ChatStream (streaming).
func (p *GeminiProvider) Stream(ctx context.Context, msgs []model.Message, emit func(delta string) error) error {

	systemText, contents := extractSystemPrompt(msgs)

	cfg := &genai.GenerateContentConfig{
		// 关键：把 System 指令放在它该在的位置
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemText}},
		},
		// 顺便把 Grounding 加上，如果你想的话
		Tools: []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}},
	}

	iter := p.Client.Models.GenerateContentStream(ctx, p.Model, contents, cfg)
	for resp, err := range iter {
		if err != nil {
			return err
		}
		// Same pattern as the official example prints each chunk's text part. :contentReference[oaicite:4]{index=4}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
			continue
		}
		delta := resp.Candidates[0].Content.Parts[0].Text
		if delta == "" {
			continue
		}
		if err := emit(delta); err != nil {
			return err
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
