package agent

import (
	"context"
	"testing"

	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"
)

func TestService_Chat_PersistsMessagesAndReturnsReply(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	conv := "c1"

	// Seed history (so builder has something to include)
	_, _ = st.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "old"}, nil)
	_, _ = st.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleAssistant, Content: "old-reply"}, nil)

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	fpClaude := &llm.FakeProvider{Provider: llm.ProviderClaude, Reply: "assistant says hi"}
	r := &router.Router{
		Claude: fpClaude,
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	svc := &Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	reply, conv, err := svc.Chat(ctx, conv, "```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if reply != "assistant says hi" {
		t.Fatalf("expected reply %q, got %q", "assistant says hi", reply)
	}

	// Verify provider received context (system prompt should be first)
	if len(fpClaude.LastMessages) == 0 {
		t.Fatalf("expected provider to receive messages")
	}
	if fpClaude.LastMessages[0].Role != model.RoleSystem {
		t.Fatalf("expected first message RoleSystem, got %#v", fpClaude.LastMessages[0])
	}

	// Verify store now has appended user+assistant messages at end
	got, err := st.GetMessagesByConvID(ctx, conv, 0, "desc", "")
	if err != nil {
		t.Fatalf("GetRecentMessages error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 total messages, got %d: %#v", len(got), got)
	}
	if got[2].Role != model.RoleUser {
		t.Fatalf("expected [2] user, got %#v", got[2])
	}
	if got[3].Role != model.RoleAssistant || got[3].Content != "assistant says hi" {
		t.Fatalf("expected [3] assistant reply, got %#v", got[3])
	}
}

func TestRouter_ChooseChain_General_GeminiThenOpenAI(t *testing.T) {
	r := &router.Router{
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
		Claude: nil, // you said leave Claude for now
	}

	got := r.ChooseChain("hello there")
	assertProviderChain(t, got, []llm.ProviderName{
		llm.ProviderGemini,
		llm.ProviderOpenAI,
	})
}

func TestRouter_ChooseChain_Coding_NoClaude_StillGeminiThenOpenAI(t *testing.T) {
	r := &router.Router{
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
		Claude: nil,
	}

	got := r.ChooseChain("```go\nfmt.Println(\"hi\")\n```")
	assertProviderChain(t, got, []llm.ProviderName{
		llm.ProviderGemini,
		llm.ProviderOpenAI,
	})
}

func TestRouter_ChooseChain_Coding_WithClaude_ClaudeFirst(t *testing.T) {
	r := &router.Router{
		Claude: &llm.FakeProvider{Provider: llm.ProviderClaude},
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	got := r.ChooseChain("```python\nprint('x')\n```")
	assertProviderChain(t, got, []llm.ProviderName{
		llm.ProviderClaude,
		llm.ProviderGemini,
		llm.ProviderOpenAI,
	})
}

func TestService_Chat_NoProvider(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	conv := "c1"

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	r := &router.Router{
		Claude: nil,
		Gemini: nil,
		OpenAI: nil,
	}

	svc := &Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	_, _, err := svc.Chat(ctx, conv, "Tell me something")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err != ErrNoProvider {
		t.Fatalf("expected %v, got %v", ErrNoProvider, err)
	}
}

func assertProviderChain(t *testing.T, got []llm.Provider, want []llm.ProviderName) {
	t.Helper()

	// Filter nil providers so the test is robust even if Router skips nils later.
	filtered := make([]llm.Provider, 0, len(got))
	for _, p := range got {
		if p != nil {
			filtered = append(filtered, p)
		}
	}

	if len(filtered) != len(want) {
		t.Fatalf("expected chain len %d, got %d: %#v", len(want), len(filtered), filtered)
	}

	for i := range want {
		if filtered[i].Name() != want[i] {
			t.Fatalf("chain[%d] expected %q, got %q", i, want[i], filtered[i].Name())
		}
	}
}
