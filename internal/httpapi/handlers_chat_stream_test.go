package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rhea-backend/internal/agent"
	"rhea-backend/internal/auth"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

func TestChatStreamHandler_OK_StreamsMetadataAndDeltas(t *testing.T) {
	// 1. 模拟身份 🚀
	uid := uuid.New()
	st := store.NewMemoryStore()

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	// 2. 模拟 Provider 和 Router
	fp := &llm.FakeProvider{
		Provider: llm.ProviderGemini,
		Chunks:   []string{"he", "llo"},
	}
	r := &router.Router{
		GeminiLite:  &llm.FakeProvider{Reply: "SIMPLE"}, // 确保路由能通
		GeminiFlash: fp,
	}
	svc := &agent.Service{Store: st, Builder: b, Router: r}

	h := &ChatStreamHandler{Agent: svc, Limit: 1024 * 1024}

	body := bytes.NewBufferString(`{"conversation_id":"","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat/stream", body)

	// 3. 注入 Auth Context 🚀
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}

	out := w.Body.String()

	// 4. 验证是否发送了模型元数据 (我们之前在 Service 增加的逻辑)
	if !strings.Contains(out, "event: delta\ndata: ::__metadata__:model:gemini:fake-v1::") {
		t.Errorf("expected metadata event, got: %q", out)
	}

	// 5. 验证内容增量
	if !strings.Contains(out, "event: delta\ndata: he\n\n") {
		t.Fatalf("expected first delta, got: %q", out)
	}
	if !strings.Contains(out, "event: delta\ndata: llo\n\n") {
		t.Fatalf("expected second delta, got: %q", out)
	}

	// 6. 验证结束标识
	if !strings.Contains(out, "event: done\ndata: [DONE]\n\n") {
		t.Fatalf("expected done event, got: %q", out)
	}
}

func TestChatStreamHandler_NoProvider_EmitsErrorEvent(t *testing.T) {
	uid := uuid.New()
	st := store.NewMemoryStore()

	// 空的 Router 会导致 ErrNoProvider
	r := &router.Router{}
	svc := &agent.Service{Store: st, Router: r, Builder: &ctxbuilder.Builder{Store: st}}

	h := &ChatStreamHandler{Agent: svc}

	body := bytes.NewBufferString(`{"message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat/stream", body)
	req = req.WithContext(auth.SetUserID(req.Context(), uid))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	out := w.Body.String()
	if !strings.Contains(out, "event: error") {
		t.Fatalf("expected error event, got: %q", out)
	}
}
