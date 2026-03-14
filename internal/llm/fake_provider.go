package llm

import (
	"context"

	"rhea-backend/internal/model"
)

type FakeProvider struct {
	// 基础配置
	ProviderName ProviderName // 对应 Name()
	Model        string       // 对应 ModelName()

	// 模拟行为
	Reply  string   // 简单回复
	Chunks []string // 流式回复的碎片
	Err    error    // 模拟报错

	// 模拟账单 (重点：让测试可以自定义这次花了多少 Token)
	Usage *Usage

	// 拦截器 (用于断言 Service 是否传对了消息)
	LastMessages []model.Message
}

func (p *FakeProvider) Name() ProviderName {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return ProviderGemini
}

func (p *FakeProvider) ModelName() string {
	if p.Model != "" {
		return p.Model
	}
	return "fake-model-v1"
}

// Chat 模拟非流式调用
func (p *FakeProvider) Chat(ctx context.Context, messages []model.Message) (ChatResponse, error) {
	p.LastMessages = messages
	if p.Err != nil {
		return ChatResponse{}, p.Err
	}

	reply := p.Reply
	if reply == "" {
		reply = "fake reply"
	}

	// 构造 Usage：优先用手动注入的，没有就给个默认值
	u := Usage{InputTokens: 10, OutputTokens: len(reply) / 4, ModelName: p.ModelName()}
	if p.Usage != nil {
		u = *p.Usage
	}

	return ChatResponse{
		Content: reply,
		Usage:   u,
	}, nil
}

// Stream 模拟流式调用 (严格对齐 GeminiProvider 的逻辑)
func (p *FakeProvider) Stream(ctx context.Context, messages []model.Message, emit func(delta string, usage *Usage) error) error {
	p.LastMessages = messages
	if p.Err != nil {
		return p.Err
	}

	// 准备 Usage 收据
	finalUsage := p.Usage
	if finalUsage == nil {
		finalUsage = &Usage{
			InputTokens:  10,
			OutputTokens: 5,
			ModelName:    p.ModelName(),
		}
	}

	// 场景 A：如果有 Chunks，逐个模拟发送
	if len(p.Chunks) > 0 {
		for i, c := range p.Chunks {
			var u *Usage
			// 模仿 Gemini：只有最后一次迭代才带上 Usage
			if i == len(p.Chunks)-1 {
				u = finalUsage
			}
			if err := emit(c, u); err != nil {
				return err
			}
		}
		return nil
	}

	// 场景 B：只有单个 Reply
	reply := p.Reply
	if reply == "" {
		reply = "fake stream reply"
	}
	// 直接发出去，并带上 Usage
	return emit(reply, finalUsage)
}
