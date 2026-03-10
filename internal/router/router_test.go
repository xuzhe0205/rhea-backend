package router

import (
	"context"
	"testing"

	"rhea-backend/internal/llm"
)

func TestRouter_ChooseChain_CodingHeuristic(t *testing.T) {
	// 1. 强特征抢跑测试：包含代码块
	r := &Router{
		GeminiPro:   &llm.FakeProvider{Reply: "I am Pro"},
		GeminiFlash: &llm.FakeProvider{Reply: "I am Flash"},
		GeminiLite:  &llm.FakeProvider{Reply: "SIMPLE"}, // 即使 Lite 返回 SIMPLE，强特征也应覆盖它
	}

	ctx := context.Background()
	// 包含 ``` 代码块
	chain := r.ChooseChain(ctx, "How to fix this: ```go\nfmt.Println(x)\n```")

	if len(chain) == 0 || chain[0] != r.GeminiPro {
		t.Errorf("Expected GeminiPro for coding heuristic, got %v", chain[0])
	}
}

func TestRouter_ChooseChain_SimpleIntent(t *testing.T) {
	// 2. 模拟 Lite 分类为 SIMPLE
	pLite := &llm.FakeProvider{Reply: "SIMPLE"}
	pFlash := &llm.FakeProvider{Reply: "Flash Reply"}
	pPro := &llm.FakeProvider{Reply: "Pro Reply"}

	r := &Router{
		GeminiLite:  pLite,
		GeminiFlash: pFlash,
		GeminiPro:   pPro,
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "Hello, how are you?")

	if len(chain) < 2 {
		t.Fatalf("Expected chain of at least 2 providers, got %d", len(chain))
	}

	// 简单问题：Flash 应该在第一位
	if chain[0] != pFlash {
		t.Errorf("Expected GeminiFlash first for SIMPLE intent, got %v", chain[0])
	}
}

func TestRouter_ChooseChain_DeepIntent(t *testing.T) {
	// 3. 模拟 Lite 分类为 DEEP
	pLite := &llm.FakeProvider{Reply: "DEEP"}
	pFlash := &llm.FakeProvider{Reply: "Flash Reply"}
	pPro := &llm.FakeProvider{Reply: "Pro Reply"}

	r := &Router{
		GeminiLite:  pLite,
		GeminiFlash: pFlash,
		GeminiPro:   pPro,
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "Explain the difference between JWT and OAuth2 in detail.")

	if len(chain) < 2 {
		t.Fatalf("Expected chain of at least 2 providers, got %d", len(chain))
	}

	// 深度问题：Pro 应该在第一位
	if chain[0] != pPro {
		t.Errorf("Expected GeminiPro first for DEEP intent, got %v", chain[0])
	}
}

func TestRouter_ChooseChain_LiteFailureFallback(t *testing.T) {
	// 4. 模拟 Lite 模型挂了的情况
	pLite := &llm.FakeProvider{Err: context.DeadlineExceeded}
	pPro := &llm.FakeProvider{Reply: "Pro"}

	r := &Router{
		GeminiLite: pLite,
		GeminiPro:  pPro,
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "Normal question")

	// 此时逻辑应该进入保底：IntentDeep -> Pro 优先
	if len(chain) == 0 || chain[0] != pPro {
		t.Errorf("Expected fallback to GeminiPro when Lite fails, got %v", chain[0])
	}
}

func TestRouter_Choose_InternalTask(t *testing.T) {
	// 5. 测试原有静态 Choose 逻辑（用于标题生成等）
	pFlash := &llm.FakeProvider{Reply: "Flash"}
	r := &Router{GeminiFlash: pFlash}

	p := r.Choose("internal_task")
	if p != pFlash {
		t.Errorf("Expected GeminiFlash for internal task, got %v", p)
	}
}
