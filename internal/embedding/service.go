package embedding

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	Provider Provider
}

func (s *Service) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if s == nil || s.Provider == nil {
		return nil, fmt.Errorf("embedding provider is not configured")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embedding input is empty")
	}

	return s.Provider.Embed(ctx, text)
}

func (s *Service) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if s == nil || s.Provider == nil {
		return nil, fmt.Errorf("embedding provider is not configured")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding inputs are empty")
	}

	return s.Provider.EmbedBatch(ctx, texts)
}
