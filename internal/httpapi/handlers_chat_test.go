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
	uid := uuid.New()
	st := store.NewMemoryStore()
	convID := uuid.New()

	_, _ = st.CreateConversation(context.Background(), &model.Conversation{
		ID:     convID,
		UserID: uid,
	})

	b := &ctxbuilder.Builder{Store: st, SystemPrompt: "You are Rhea."}

	fp := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Reply:        "hi from fake",
		Model:        "fake-model-v1",
	}

	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "SIMPLE"},
		GeminiFlash: fp,
		GeminiPro:   fp,
	}

	svc := &agent.Service{Store: st, Builder: b, Router: r}
	h := &ChatHandler{Agent: svc}

	body := bytes.NewBufferString(`{"conversation_id":"` + convID.String() + `","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
}

func TestChatHandler_NoProvider_Returns503(t *testing.T) {
	uid := uuid.New()
	st := store.NewMemoryStore()
	convID := uuid.New()
	_, _ = st.CreateConversation(context.Background(), &model.Conversation{ID: convID, UserID: uid})

	// 🚀 核心修复：NoProvider 测试也需要有效的 Builder 配置
	b := &ctxbuilder.Builder{
		Store:        st,
		SystemPrompt: "You are Rhea.", // 加上这个，避开 Builder 的校验报错
	}

	// Router 为空
	r := &router.Router{}

	svc := &agent.Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	h := &ChatHandler{Agent: svc}

	body := bytes.NewBufferString(`{"conversation_id":"` + convID.String() + `","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
}

func TestChatHandler_ListConversations_OK(t *testing.T) {
	uid := uuid.New()
	st := store.NewMemoryStore()

	_, _ = st.CreateConversation(context.Background(), &model.Conversation{ID: uuid.New(), UserID: uid, Title: "Conv 1"})
	_, _ = st.CreateConversation(context.Background(), &model.Conversation{ID: uuid.New(), UserID: uid, Title: "Conv 2"})

	svc := &agent.Service{Store: st, Router: &router.Router{}}
	h := &ChatHandler{Agent: svc}

	req := httptest.NewRequest(http.MethodGet, "/v1/conversations", nil)
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()
	h.ListConversations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Conv 1") || !strings.Contains(w.Body.String(), "Conv 2") {
		t.Errorf("expected conversations in response, got %q", w.Body.String())
	}
}
