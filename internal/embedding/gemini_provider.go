package embedding

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

type GeminiEmbeddingProvider struct {
	Model  string
	Client *genai.Client
}

func NewGeminiEmbeddingProvider(ctx context.Context, apiKey string, model string) (*GeminiEmbeddingProvider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(model) == "" {
		model = "gemini-embedding-001"
	}

	return &GeminiEmbeddingProvider{
		Model:  model,
		Client: client,
	}, nil
}

func (p *GeminiEmbeddingProvider) ModelName() string {
	return p.Model
}

func (p *GeminiEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embedding input is empty")
	}

	dim := int32(1536)

	resp, err := p.Client.Models.EmbedContent(
		ctx,
		p.Model,
		[]*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{Text: text},
				},
			},
		},
		&genai.EmbedContentConfig{
			OutputDimensionality: &dim,
		},
	)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("nil embedding response")
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("embedding response contains no embeddings")
	}
	if resp.Embeddings[0] == nil {
		return nil, fmt.Errorf("embedding response contains nil embedding")
	}
	if len(resp.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("embedding vector is empty")
	}

	out := make([]float32, len(resp.Embeddings[0].Values))
	copy(out, resp.Embeddings[0].Values)
	return out, nil
}

func (p *GeminiEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding inputs are empty")
	}

	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		v, err := p.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
