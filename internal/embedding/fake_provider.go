package embedding

import "context"

type FakeProvider struct{}

func (p *FakeProvider) ModelName() string {
	return "fake"
}

func (p *FakeProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

func (p *FakeProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for range texts {
		out = append(out, []float32{0.1, 0.2, 0.3, 0.4})
	}
	return out, nil
}
