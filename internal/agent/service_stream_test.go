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

func TestServiceStream_FailoverAndMetadata(t *testing.T) {
	// 1. 环境准备
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid) // 使用你已有的 SetUserID
	st := store.NewMemoryStore()

	// 2. 模拟场景：第一个 Provider 会爆炸，第二个 Provider 才是对的
	pFail := &llm.FakeProvider{
		Provider: llm.ProviderGemini,
		Err:      errors.New("connection reset by peer"),
	}
	pSuccess := &llm.FakeProvider{
		Provider: llm.ProviderGemini,
		Reply:    "I am the backup, and I work!",
	}

	// 构造 Router：让 Flash 先上（失败），Pro 后上（成功）
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

	// 3. 执行：预期应该是成功的，因为有 Failover 机制
	convID, err := svc.ChatStream(ctx, "", "Can you hear me?", emit)
	if err != nil {
		t.Fatalf("ChatStream failed despite failover: %v", err)
	}

	// 4. 断言：元数据是否正确
	// 预期会 emit 两次 metadata（第一次失败的，第二次成功的）或者至少有最后一次成功的
	foundSuccessModel := false
	fullResponse := ""
	for _, p := range emittedParts {
		if strings.Contains(p, "fake-model-v1") {
			foundSuccessModel = true
		} else if !strings.HasPrefix(p, "::__metadata__:") {
			fullResponse += p
		}
	}

	if !foundSuccessModel {
		t.Error("Metadata for the successful provider was not emitted")
	}
	if fullResponse != "I am the backup, and I work!" {
		t.Errorf("Unexpected response: %s", fullResponse)
	}

	// 5. 断言持久化
	msgs, _ := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	hasAssistantMsg := false
	for _, m := range msgs {
		if m.Role == model.RoleAssistant {
			hasAssistantMsg = true
		}
	}
	if !hasAssistantMsg {
		t.Error("Assistant message was not persisted after successful failover")
	}
}

func TestServiceStream_UserMessagePersistence(t *testing.T) {
	// 这个用例专门测试即便全部报错，用户的提问也要存下来
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

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

	// 即使失败，消息列表里应该有且只有那条 User 消息
	msgs, _ := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if len(msgs) == 0 || msgs[0].Role != model.RoleUser {
		t.Error("User message should be persisted even if AI fails")
	}
}
