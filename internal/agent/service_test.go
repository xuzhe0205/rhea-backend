package agent

import (
	"context"
	"testing"

	"rhea-backend/internal/auth"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

func TestService_Chat_PersistsMessagesAndReturnsReply(t *testing.T) {
	// 1. 模拟身份 🚀
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)

	st := store.NewMemoryStore()
	convID := uuid.New().String()

	// 2. 预设对话和历史记录 (MemoryStore 需要先有这个对话，否则 GetConversation 会报错)
	_, _ = st.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})
	_, _ = st.AppendMessage(ctx, convID, nil, model.Message{Role: model.RoleUser, Content: "old"}, nil)

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	// 3. 构造符合新 Router 结构的 Mock
	fpPro := &llm.FakeProvider{Provider: llm.ProviderGemini, Reply: "assistant says hi"}
	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "DEEP"}, // 路由选择逻辑
		GeminiPro:   fpPro,
		GeminiFlash: &llm.FakeProvider{Reply: "flash reply"},
	}

	svc := &Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	// 4. 执行
	reply, returnedConvID, err := svc.Chat(ctx, convID, "How to write Go?")
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if reply != "assistant says hi" {
		t.Fatalf("expected reply %q, got %q", "assistant says hi", reply)
	}
	if returnedConvID != convID {
		t.Errorf("expected convID %s, got %s", convID, returnedConvID)
	}

	// 5. 验证 Store 状态 (由于 st.AppendMessage 逻辑，预期有 3 条：old, user, assistant)
	got, err := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}

	// 检查最后一条是否是 AI 回复
	lastMsg := got[len(got)-1]
	if lastMsg.Role != model.RoleAssistant || lastMsg.Content != "assistant says hi" {
		t.Errorf("Last message persistent error, got: %+v", lastMsg)
	}
}

func TestService_Chat_NoProvider(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

	// 预设对话
	convID := uuid.New().String()
	_, _ = st.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})

	b := &ctxbuilder.Builder{Store: st, SystemPrompt: "You are Rhea."}

	// 所有 Provider 都是 nil
	r := &router.Router{
		GeminiPro:   nil,
		GeminiFlash: nil,
		GeminiLite:  nil,
	}

	svc := &Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	_, _, err := svc.Chat(ctx, convID, "Tell me something")
	if err != ErrNoProvider {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

// 🚀 针对新的 ChooseChain 逻辑进行测试
func TestRouter_ChooseChain_NewLogic(t *testing.T) {
	pPro := &llm.FakeProvider{Provider: llm.ProviderGemini}
	pFlash := &llm.FakeProvider{Provider: llm.ProviderGemini}
	pLite := &llm.FakeProvider{Provider: llm.ProviderGemini}

	r := &router.Router{
		GeminiPro:   pPro,
		GeminiFlash: pFlash,
		GeminiLite:  pLite,
	}

	// 在你的 router.go 实现中，ChooseChain 应该返回具体的优先级
	// 这里我们简单测试它是否返回了非空的链条
	ctx := context.Background()
	got := r.ChooseChain(ctx, "hello")

	if len(got) == 0 {
		t.Fatal("expected a non-empty provider chain")
	}
}
