package agent

import (
	"context"
	"fmt"
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

	// 2. 预设对话
	_, _ = st.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})
	_, _ = st.AppendMessage(ctx, convID, nil, model.Message{Role: model.RoleUser, Content: "old"}, nil)

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	// 3. 构造符合新架构的 Mock
	fpPro := &llm.FakeProvider{Model: "gemini-pro", Reply: "assistant says hi"}
	fpFree := &llm.FakeProvider{Model: "flash-free", Reply: "flash free reply"}
	fpPaid := &llm.FakeProvider{Model: "flash-paid", Reply: "flash paid reply"}

	r := &router.Router{
		GeminiLite:      &llm.FakeProvider{Reply: "DEEP"}, // 模拟深度意图
		GeminiPro:       fpPro,
		GeminiFlashFree: fpFree,
		GeminiFlash:     fpPaid,
	}

	svc := &Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	// 4. 执行 (DEEP 意图应返回 Pro 的回复)
	reply, returnedConvID, err := svc.Chat(ctx, convID, "How to write Go?")
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	// 5. 验证回复
	if reply != "assistant says hi" {
		t.Fatalf("expected reply from Pro %q, got %q", "assistant says hi", reply)
	}
	if returnedConvID != convID {
		t.Errorf("expected convID %s, got %s", convID, returnedConvID)
	}

	// 6. 验证 Store 状态
	got, _ := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	lastMsg := got[len(got)-1]
	if lastMsg.Role != model.RoleAssistant || lastMsg.Content != "assistant says hi" {
		t.Errorf("Last message persistent error, got: %+v", lastMsg)
	}

	// 验证 Token 统计
	conv, _ := st.GetConversation(ctx, convID)
	if conv.CumulativeTokens <= 0 {
		t.Errorf("CumulativeTokens should be greater than 0, got %d", conv.CumulativeTokens)
	}
}

func TestService_Chat_Failover_FreeToPaid(t *testing.T) {
	// 🚀 核心测试：验证 Free Tier 挂了能不能自动切到 Paid
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()
	convID := uuid.New().String()
	_, _ = st.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})

	b := &ctxbuilder.Builder{Store: st, SystemPrompt: "You are Rhea."}

	// 模拟 Free 报错，Paid 正常
	pFree := &llm.FakeProvider{Model: "flash-free", Err: fmt.Errorf("429 resource exhausted")}
	pPaid := &llm.FakeProvider{Model: "flash-paid", Reply: "recovered by paid tier"}

	r := &router.Router{
		GeminiLite:      &llm.FakeProvider{Reply: "SIMPLE"},
		GeminiFlashFree: pFree,
		GeminiFlash:     pPaid,
		GeminiPro:       &llm.FakeProvider{Model: "pro"},
	}

	svc := &Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	// 执行：SIMPLE 意图会先尝试 Free，失败后应尝试 Paid
	reply, _, err := svc.Chat(ctx, convID, "Simple task")
	if err != nil {
		t.Fatalf("Expected failover success, got error: %v", err)
	}

	if reply != "recovered by paid tier" {
		t.Errorf("Failover logic failed, got reply: %q", reply)
	}
}

func TestService_Chat_NoProvider(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()
	convID := uuid.New().String()
	_, _ = st.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})

	b := &ctxbuilder.Builder{Store: st, SystemPrompt: "You are Rhea."}

	r := &router.Router{
		GeminiPro:       nil,
		GeminiFlash:     nil,
		GeminiFlashFree: nil,
		GeminiLite:      nil,
	}

	svc := &Service{Store: st, Builder: b, Router: r}

	_, _, err := svc.Chat(ctx, convID, "Tell me something")
	if err != ErrNoProvider {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

func TestRouter_ChooseChain_NewLogic(t *testing.T) {
	pPro := &llm.FakeProvider{Model: "gemini-pro"}
	pFlashFree := &llm.FakeProvider{Model: "flash-free"}
	pFlashPaid := &llm.FakeProvider{Model: "flash-paid"}
	pLite := &llm.FakeProvider{Reply: "SIMPLE"}

	r := &router.Router{
		GeminiPro:       pPro,
		GeminiFlashFree: pFlashFree,
		GeminiFlash:     pFlashPaid,
		GeminiLite:      pLite,
	}

	ctx := context.Background()

	// 1. 测试代码块拦截 (Pro -> Free -> Paid)
	gotCoding := r.ChooseChain(ctx, "Check: ```go ... ```")
	if len(gotCoding) == 0 || gotCoding[0].ModelName() != "gemini-pro" {
		t.Errorf("Expected Pro first for coding")
	}

	// 2. 测试简单意图 (Free -> Paid -> Pro)
	gotSimple := r.ChooseChain(ctx, "Hello")
	if len(gotSimple) < 2 || gotSimple[0].ModelName() != "flash-free" {
		t.Errorf("Expected FlashFree first for simple intent")
	}
	if gotSimple[1].ModelName() != "flash-paid" {
		t.Errorf("Expected FlashPaid to be the second choice (backup)")
	}
}
