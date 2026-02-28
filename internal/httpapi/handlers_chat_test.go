package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"rhea-backend/internal/agent"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"
)

func TestChatHandler_PostOK(t *testing.T) {
	st := store.NewMemoryStore()
	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}
	fp := &llm.FakeProvider{Provider: llm.ProviderGemini, Reply: "hi from fake"}
	r := &router.Router{Gemini: fp}

	svc := &agent.Service{Store: st, Builder: b, Router: r}
	h := &ChatHandler{Agent: svc}

	body := bytes.NewBufferString(`{"conversation_id":"c1","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got == "" || got[0] != '{' {
		t.Fatalf("expected json response, got %q", got)
	}
}

func TestChatHandler_BadRequest(t *testing.T) {
	h := &ChatHandler{Agent: &agent.Service{}}

	body := bytes.NewBufferString(`{"conversation_id":"","message":""}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", w.Code, w.Body.String())
	}
}

func TestChatHandler_MethodNotAllowed(t *testing.T) {
	h := &ChatHandler{}
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestChatHandler_NoProvider_Returns503(t *testing.T) {
	st := store.NewMemoryStore()
	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	// Router has NO providers -> agent will return agent.ErrNoProvider
	r := &router.Router{
		Claude: nil,
		Gemini: nil,
		OpenAI: nil,
	}

	svc := &agent.Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	h := &ChatHandler{Agent: svc}

	body := bytes.NewBufferString(`{"conversation_id":"c1","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
}

func TestChatHandler_PayloadTooLarge_Returns413(t *testing.T) {
	// We don't actually need Agent to be configured because decoding fails first,
	// but provide a non-nil handler anyway.
	h := &ChatHandler{Agent: &agent.Service{}}

	// Build a JSON body > 1MB.
	// We'll keep it valid JSON so we specifically test MaxBytesReader behavior.
	tooBig := make([]byte, 1024*1024+10) // 1MB + 10 bytes
	for i := range tooBig {
		tooBig[i] = 'a'
	}

	// Construct valid JSON: {"conversation_id":"c1","message":"aaaa...."}
	body := bytes.NewBuffer(nil)
	body.WriteString(`{"conversation_id":"c1","message":"`)
	body.Write(tooBig)
	body.WriteString(`"}`)

	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%q", w.Code, w.Body.String())
	}
}
