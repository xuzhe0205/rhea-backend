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

	// 2. 预设对话
	_, _ = st.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})
	// 注意：AppendMessage 签名如果改了，这里也要带上最后的 nil 或 0
	_, _ = st.AppendMessage(ctx, convID, nil, model.Message{Role: model.RoleUser, Content: "old"}, nil)

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	// 3. 构造符合新 Provider 接口的 Mock
	// 此时 FakeProvider.Chat 会返回 ChatResponse 结构体
	fpPro := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Reply:        "assistant says hi",
	}

	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "DEEP"}, // 路由意图识别
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

	// 5. 验证回复
	if reply != "assistant says hi" {
		t.Fatalf("expected reply %q, got %q", "assistant says hi", reply)
	}
	if returnedConvID != convID {
		t.Errorf("expected convID %s, got %s", convID, returnedConvID)
	}

	// 6. 验证 Store 状态
	got, err := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}

	// 验证最后一条消息的 Role 和内容
	lastMsg := got[len(got)-1]
	if lastMsg.Role != model.RoleAssistant || lastMsg.Content != "assistant says hi" {
		t.Errorf("Last message persistent error, got: %+v", lastMsg)
	}

	// 🚀 新增验证：验证 Token 统计是否生效
	conv, _ := st.GetConversation(ctx, convID)
	// 因为 FakeProvider 默认会 mock 一些 token (比如 10+5)
	if conv.CumulativeTokens <= 0 {
		t.Errorf("CumulativeTokens should be greater than 0, got %d", conv.CumulativeTokens)
	}
}

func TestService_Chat_NoProvider(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

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

func TestRouter_ChooseChain_NewLogic(t *testing.T) {
	// 为测试准备 mock 数据
	pPro := &llm.FakeProvider{ProviderName: llm.ProviderGemini, Reply: "pro"}
	pFlash := &llm.FakeProvider{ProviderName: llm.ProviderGemini, Reply: "flash"}
	pLite := &llm.FakeProvider{ProviderName: llm.ProviderGemini, Reply: "SIMPLE"} // 返回简单意图

	r := &router.Router{
		GeminiPro:   pPro,
		GeminiFlash: pFlash,
		GeminiLite:  pLite,
	}

	ctx := context.Background()

	// 测试包含代码块的强特征拦截
	gotCoding := r.ChooseChain(ctx, "Check this code: ```go ... ```")
	if len(gotCoding) == 0 || gotCoding[0] != pPro {
		t.Errorf("Expected Pro to be first for coding heuristic")
	}

	// 测试普通文本（经由 Lite 分类）
	gotSimple := r.ChooseChain(ctx, "Hello")
	if len(gotSimple) == 0 || gotSimple[0] != pFlash {
		t.Errorf("Expected Flash to be first for SIMPLE intent")
	}
}
