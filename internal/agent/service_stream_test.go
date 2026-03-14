package agent

import (
	"context"
	"errors"
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

	// 2. 模拟场景：第一个 Provider 故障，第二个成功并返回 Token 消耗
	// 🚀 对齐字段：ProviderName, Model
	pFail := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Model:        "flash-v1",
		Err:          errors.New("connection reset"),
	}

	// 成功者带上具体的 Usage 消耗
	pSuccess := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Model:        "pro-v2",
		Reply:        "I am the backup, and I work!",
		// 🚀 模拟收据：输入 40，输出 60，总计 100
		Usage: &llm.Usage{
			InputTokens:  40,
			OutputTokens: 60,
			ModelName:    "pro-v2",
		},
	}

	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "SIMPLE"},
		GeminiFlash: pFail,
		GeminiPro:   pSuccess,
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

	// 3. 执行
	convID, err := svc.ChatStream(ctx, "", "Can you hear me?", emit)
	if err != nil {
		t.Fatalf("ChatStream failed despite failover: %v", err)
	}

	// 4. 断言元数据和内容
	foundModelMetadata := false
	fullResponse := ""
	for _, p := range emittedParts {
		// 验证 metadata 是否包含了正确的 Provider 名称
		if strings.Contains(p, string(llm.ProviderGemini)) {
			foundModelMetadata = true
		}
		// 过滤掉 metadata 前缀，拼接实际回复
		if !strings.HasPrefix(p, "::__metadata__:") {
			fullResponse += p
		}
	}

	if !foundModelMetadata {
		t.Error("Metadata for the successful provider was not emitted")
	}
	if fullResponse != "I am the backup, and I work!" {
		t.Errorf("Unexpected response content: %s", fullResponse)
	}

	// 5. 🚀 关键断言：Token 账单原子累加是否成功
	conv, _ := st.GetConversation(ctx, convID)
	// 预期 CumulativeTokens 从 0 变成 100
	if conv.CumulativeTokens != 100 {
		t.Errorf("Token accounting failed: expected 100, got %d", conv.CumulativeTokens)
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
	// 验证单条消息级别的 Token 记录
	if assistantMsg.InputTokens != 40 || assistantMsg.OutputTokens != 60 {
		t.Errorf("Message-level tokens error: got %d/%d, expected 40/60", assistantMsg.InputTokens, assistantMsg.OutputTokens)
	}
}

func TestServiceStream_UserMessagePersistence(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

	// 全部失败的情况
	pFatal := &llm.FakeProvider{Err: errors.New("total blackout")}
	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "DEEP"},
		GeminiPro:   pFatal,
		GeminiFlash: pFatal,
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

	// 即使全挂了，用户消息也必须持久化（防止丢上下文）
	msgs, _ := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if len(msgs) == 0 || msgs[0].Role != model.RoleUser {
		t.Error("User message should be persisted even if AI fails")
	}

	// 失败时，不产生 Token 费用
	conv, _ := st.GetConversation(ctx, convID)
	if conv.CumulativeTokens != 0 {
		t.Errorf("Tokens should be 0 on total failure, got %d", conv.CumulativeTokens)
	}
}
