package llm

import (
	"context"

	"rhea-backend/internal/model"
)

type FakeProvider struct {
	Provider     ProviderName
	Reply        string
	Chunks       []string
	Err          error
	LastMessages []model.Message // captured input for assertions
}

func (p *FakeProvider) Name() ProviderName {
	return p.Provider
}

func (p *FakeProvider) Chat(ctx context.Context, messages []model.Message) (string, error) {
	p.LastMessages = messages

	if p.Err != nil {
		return "", p.Err
	}
	if p.Reply == "" {
		return string(p.Provider) + ":ok", nil
	}
	return p.Reply, nil
}

func (p *FakeProvider) Stream(ctx context.Context, messages []model.Message, emit func(delta string) error) error {
	p.LastMessages = messages
	if p.Err != nil {
		return p.Err
	}

	// If Chunks provided, stream those; otherwise stream Reply once.
	if len(p.Chunks) > 0 {
		for _, c := range p.Chunks {
			if err := emit(c); err != nil {
				return err
			}
		}
		return nil
	}

	reply := p.Reply
	if reply == "" {
		reply = string(p.Provider) + ":ok"
	}
	return emit(reply)
}
