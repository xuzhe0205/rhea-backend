package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"rhea-backend/internal/auth"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

func TestServiceStream_FailoverAndTokenAccounting(t *testing.T) {
	// 1. 环境准备
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

	// 2. 模拟场景：Free Tier 报 429 错误，Paid Tier 成功补位
	// 第一个：免费版，返回 429 错误
	pFreeFail := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Model:        "flash-free-v3",
		Err:          fmt.Errorf("429 RESOURCE_EXHAUSTED: quota exceeded"),
	}

	// 第二个：付费版，成功并返回 Token 消耗 (40 + 60 = 100)
	pPaidSuccess := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Model:        "flash-paid-v3",
		Reply:        "I am the paid backup, and I work!",
		Usage: &llm.Usage{
			InputTokens:  40,
			OutputTokens: 60,
			ModelName:    "flash-paid-v3",
		},
	}

	r := &router.Router{
		// 🚀 关键修复：将 Lite 的消耗设为 0，避免干扰最终 CumulativeTokens 的断言
		GeminiLite: &llm.FakeProvider{
			Reply: "SIMPLE",
			Usage: &llm.Usage{InputTokens: 0, OutputTokens: 0},
		},
		GeminiFlashFree: pFreeFail,
		GeminiFlash:     pPaidSuccess,
		GeminiPro:       &llm.FakeProvider{Model: "pro-v2"},
	}

	svc := &Service{
		Store:   st,
		Builder: &ctxbuilder.Builder{Store: st, SystemPrompt: "Hi"},
		Router:  r,
	}

	var emittedParts []string
	emit := func(delta string) error {
		emittedParts = append(emittedParts, delta)
		return nil
	}

	// 3. 执行：由于是 SIMPLE 意图，会进入 Free -> Paid 链路
	convID, err := svc.ChatStream(ctx, "", "Can you hear me?", emit)
	if err != nil {
		t.Fatalf("ChatStream failed despite failover: %v", err)
	}

	// 4. 断言：验证元数据和内容
	foundPaidMetadata := false
	fullResponse := ""
	for _, p := range emittedParts {
		if strings.Contains(p, "flash-paid-v3") {
			foundPaidMetadata = true
		}
		if !strings.HasPrefix(p, "::__metadata__:") {
			fullResponse += p
		}
	}

	if !foundPaidMetadata {
		t.Error("Metadata for the PAID provider was not emitted after failover")
	}
	if fullResponse != "I am the paid backup, and I work!" {
		t.Errorf("Unexpected response content: %s", fullResponse)
	}

	// 5. 🚀 关键断言：验证 Token 统计 (应精确等于 100)
	conv, _ := st.GetConversation(ctx, convID)
	expectedMainReplyTokens := 40 + 60
	expectedTitleTokens := 100 // or whatever your fake internal-task provider returns
	expectedTotal := expectedMainReplyTokens + expectedTitleTokens

	if conv.CumulativeTokens != expectedTotal {
		t.Errorf("Token accounting failed: expected %d, got %d", expectedTotal, conv.CumulativeTokens)
	}

	// 6. 验证消息详情持久化
	msgs, _ := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	var assistantMsg *model.Message
	for _, m := range msgs {
		if m.Role == model.RoleAssistant {
			assistantMsg = &m
			break
		}
	}

	if assistantMsg == nil {
		t.Fatal("Assistant message was not persisted")
	}
	if assistantMsg.InputTokens != 40 || assistantMsg.OutputTokens != 60 {
		t.Errorf("Message-level tokens error: got %d/%d, expected 40/60", assistantMsg.InputTokens, assistantMsg.OutputTokens)
	}
}

func TestServiceStream_UserMessagePersistence(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

	// 全部失败的情况
	pFatal := &llm.FakeProvider{Err: fmt.Errorf("network unreachable")}
	r := &router.Router{
		GeminiLite:      &llm.FakeProvider{Reply: "DEEP", Usage: &llm.Usage{}},
		GeminiPro:       pFatal,
		GeminiFlashFree: pFatal,
		GeminiFlash:     pFatal,
	}

	svc := &Service{
		Store:   st,
		Builder: &ctxbuilder.Builder{Store: st},
		Router:  r,
	}

	convID, err := svc.ChatStream(ctx, "", "Persistence test", func(string) error { return nil })
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// 用户消息即使在 AI 崩溃时也应持久化
	msgs, _ := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if len(msgs) == 0 || msgs[0].Role != model.RoleUser {
		t.Error("User message should be persisted even if AI fails")
	}

	conv, _ := st.GetConversation(ctx, convID)
	if conv.CumulativeTokens != 0 {
		t.Errorf("Tokens should be 0 on total failure, got %d", conv.CumulativeTokens)
	}
}
