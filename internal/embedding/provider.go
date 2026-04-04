package embedding

import "context"

type Provider interface {
	ModelName() string
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
