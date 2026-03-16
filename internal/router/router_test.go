package router

import (
	"context"
	"testing"

	"rhea-backend/internal/llm"
)

func TestRouter_ChooseChain_CodingHeuristic(t *testing.T) {
	// 1. 强特征抢跑测试：包含代码块
	// 预期顺序：Pro -> FlashFree -> Flash
	pPro := &llm.FakeProvider{Model: "gemini-pro"}
	pFlashFree := &llm.FakeProvider{Model: "flash-free"}
	pFlash := &llm.FakeProvider{Model: "flash-paid"}

	r := &Router{
		GeminiPro:       pPro,
		GeminiFlashFree: pFlashFree,
		GeminiFlash:     pFlash,
		GeminiLite:      &llm.FakeProvider{Reply: "SIMPLE"},
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "How to fix this: ```go\nfmt.Println(x)\n```")

	if len(chain) != 3 {
		t.Fatalf("Expected chain length 3, got %d", len(chain))
	}
	if chain[0].ModelName() != "gemini-pro" {
		t.Errorf("Expected gemini-pro first, got %s", chain[0].ModelName())
	}
}

func TestRouter_ChooseChain_SimpleIntent(t *testing.T) {
	// 2. 模拟 Lite 分类为 SIMPLE
	// 预期顺序：FlashFree -> Flash -> Pro
	pLite := &llm.FakeProvider{Reply: "SIMPLE"}
	pFlashFree := &llm.FakeProvider{Model: "flash-free"}
	pFlash := &llm.FakeProvider{Model: "flash-paid"}
	pPro := &llm.FakeProvider{Model: "gemini-pro"}

	r := &Router{
		GeminiLite:      pLite,
		GeminiFlashFree: pFlashFree,
		GeminiFlash:     pFlash,
		GeminiPro:       pPro,
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "Hello, how are you?")

	if len(chain) != 3 {
		t.Fatalf("Expected chain length 3, got %d", len(chain))
	}
	// 核心验证：优先使用 Free Tier
	if chain[0].ModelName() != "flash-free" {
		t.Errorf("Expected flash-free first for SIMPLE intent, got %s", chain[0].ModelName())
	}
	if chain[1].ModelName() != "flash-paid" {
		t.Errorf("Expected flash-paid second, got %s", chain[1].ModelName())
	}
}

func TestRouter_ChooseChain_DeepIntent(t *testing.T) {
	// 3. 模拟 Lite 分类为 DEEP
	// 预期顺序：Pro -> FlashFree -> Flash
	pLite := &llm.FakeProvider{Reply: "DEEP"}
	pPro := &llm.FakeProvider{Model: "gemini-pro"}
	pFlashFree := &llm.FakeProvider{Model: "flash-free"}
	pFlash := &llm.FakeProvider{Model: "flash-paid"}

	r := &Router{
		GeminiLite:      pLite,
		GeminiPro:       pPro,
		GeminiFlashFree: pFlashFree,
		GeminiFlash:     pFlash,
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "Analyze this architecture.")

	if chain[0].ModelName() != "gemini-pro" {
		t.Errorf("Expected gemini-pro first for DEEP intent, got %s", chain[0].ModelName())
	}
}

func TestRouter_ChooseChain_LiteFailureFallback(t *testing.T) {
	// 4. 模拟 Lite 模型挂了的情况 -> 逻辑应进入 IntentDeep 分支
	pLite := &llm.FakeProvider{Err: context.DeadlineExceeded}
	pPro := &llm.FakeProvider{Model: "gemini-pro"}

	r := &Router{
		GeminiLite: pLite,
		GeminiPro:  pPro,
	}

	ctx := context.Background()
	chain := r.ChooseChain(ctx, "Normal question")

	if len(chain) == 0 || chain[0].ModelName() != "gemini-pro" {
		t.Errorf("Expected fallback to gemini-pro when Lite fails, got %v", chain[0])
	}
}
