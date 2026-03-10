package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rhea-backend/internal/agent"
	"rhea-backend/internal/auth"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

func TestChatHandler_PostOK(t *testing.T) {
	// 1. 模拟身份和环境 🚀
	uid := uuid.New()
	st := store.NewMemoryStore()
	convID := uuid.New()

	// 预设对话，否则 Agent.Chat 会因为权限校验失败
	_, _ = st.CreateConversation(context.Background(), &model.Conversation{
		ID:     convID,
		UserID: uid,
	})

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	// 2. 构造符合新架构的 Router
	fp := &llm.FakeProvider{Provider: llm.ProviderGemini, Reply: "hi from fake"}
	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "SIMPLE"},
		GeminiFlash: fp,
	}

	svc := &agent.Service{Store: st, Builder: b, Router: r}
	h := &ChatHandler{Agent: svc}

	// 3. 构造请求并注入 Context 🚀
	body := bytes.NewBufferString(`{"conversation_id":"` + convID.String() + `","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req.Header.Set("Content-Type", "application/json")

	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()

	// 4. 执行
	h.ServeHTTP(w, req)

	// 5. 断言
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !strings.Contains(got, "hi from fake") {
		t.Fatalf("expected reply in json, got %q", got)
	}
}

func TestChatHandler_NoProvider_Returns503(t *testing.T) {
	uid := uuid.New()
	st := store.NewMemoryStore()
	convID := uuid.New()
	_, _ = st.CreateConversation(context.Background(), &model.Conversation{ID: convID, UserID: uid})

	// Router 为空
	r := &router.Router{}

	svc := &agent.Service{
		Store:   st,
		Builder: &ctxbuilder.Builder{Store: st},
		Router:  r,
	}

	h := &ChatHandler{Agent: svc}

	body := bytes.NewBufferString(`{"conversation_id":"` + convID.String() + `","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestChatHandler_ListConversations_OK(t *testing.T) {
	uid := uuid.New()
	st := store.NewMemoryStore()

	// 存入两个对话
	_, _ = st.CreateConversation(context.Background(), &model.Conversation{ID: uuid.New(), UserID: uid, Title: "Conv 1"})
	_, _ = st.CreateConversation(context.Background(), &model.Conversation{ID: uuid.New(), UserID: uid, Title: "Conv 2"})

	svc := &agent.Service{Store: st}
	h := &ChatHandler{Agent: svc}

	req := httptest.NewRequest(http.MethodGet, "/v1/conversations", nil)
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()
	h.ListConversations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// 简单校验是否返回了 JSON 数组
	if !strings.Contains(w.Body.String(), "Conv 1") || !strings.Contains(w.Body.String(), "Conv 2") {
		t.Errorf("expected conversations in response, got %q", w.Body.String())
	}
}
