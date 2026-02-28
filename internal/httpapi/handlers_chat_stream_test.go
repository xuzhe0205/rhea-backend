package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rhea-backend/internal/agent"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"
)

func TestChatStreamHandler_OK_StreamsDeltasAndDone(t *testing.T) {
	st := store.NewMemoryStore()
	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	fp := &llm.FakeProvider{
		Provider: llm.ProviderGemini,
		Chunks:   []string{"he", "llo"},
	}
	r := &router.Router{Gemini: fp}
	svc := &agent.Service{Store: st, Builder: b, Router: r}

	h := &ChatStreamHandler{Agent: svc, Limit: 1024 * 1024}

	body := bytes.NewBufferString(`{"conversation_id":"c1","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat/stream", body)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}

	out := w.Body.String()
	if !strings.Contains(out, "event: delta\ndata: he\n\n") {
		t.Fatalf("expected first delta, got: %q", out)
	}
	if !strings.Contains(out, "event: delta\ndata: llo\n\n") {
		t.Fatalf("expected second delta, got: %q", out)
	}
	if !strings.Contains(out, "event: done\ndata: [DONE]\n\n") {
		t.Fatalf("expected done event, got: %q", out)
	}
}

func TestChatStreamHandler_NoProvider_EmitsErrorEvent(t *testing.T) {
	st := store.NewMemoryStore()
	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}
	r := &router.Router{} // all nil providers
	svc := &agent.Service{Store: st, Builder: b, Router: r}

	h := &ChatStreamHandler{Agent: svc}

	body := bytes.NewBufferString(`{"conversation_id":"c1","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat/stream", body)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	out := w.Body.String()
	if !strings.Contains(out, "event: error") {
		t.Fatalf("expected error event, got: %q", out)
	}
}

func TestChatStreamHandler_OK_StreamsDeltasWithMultiLine(t *testing.T) {
	st := store.NewMemoryStore()
	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	fp := &llm.FakeProvider{
		Provider: llm.ProviderGemini,
		Chunks:   []string{"hello\nworld"},
	}
	r := &router.Router{Gemini: fp}
	svc := &agent.Service{Store: st, Builder: b, Router: r}

	h := &ChatStreamHandler{Agent: svc, Limit: 1024 * 1024}

	body := bytes.NewBufferString(`{"conversation_id":"c1","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat/stream", body)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}

	out := w.Body.String()
	if !strings.Contains(out, "event: delta\ndata: hello\ndata: world\n\n") {
		t.Fatalf("expected first delta, got: %q", out)
	}
}
