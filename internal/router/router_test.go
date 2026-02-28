package router

import (
	"testing"

	"rhea-backend/internal/llm"
)

func TestRouter_Choose_CodingGoesToClaude(t *testing.T) {
	r := &Router{
		Claude: &llm.FakeProvider{Provider: llm.ProviderClaude},
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	p := r.Choose("Here is my code:\n```go\nfunc main() {}\n```")
	if p == nil || p.Name() != llm.ProviderClaude {
		t.Fatalf("expected claude, got %#v", p)
	}
}

func TestRouter_Choose_GeneralGoesToGemini(t *testing.T) {
	r := &Router{
		Claude: &llm.FakeProvider{Provider: llm.ProviderClaude},
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	p := r.Choose("What's a good travel plan for Montreal in winter?")
	if p == nil || p.Name() != llm.ProviderGemini {
		t.Fatalf("expected gemini, got %#v", p)
	}
}

func TestRouter_Choose_FallbackToOpenAIIfNoGemini(t *testing.T) {
	r := &Router{
		Claude: &llm.FakeProvider{Provider: llm.ProviderClaude},
		Gemini: nil,
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	p := r.Choose("Tell me a joke")
	if p == nil || p.Name() != llm.ProviderOpenAI {
		t.Fatalf("expected openai, got %#v", p)
	}
}

func TestRouter_Choose_CodingFallsBackIfNoClaude(t *testing.T) {
	r := &Router{
		Claude: nil,
		Gemini: &llm.FakeProvider{Provider: llm.ProviderGemini},
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	p := r.Choose("I'm seeing a compile error in my Go build")
	if p == nil || p.Name() != llm.ProviderGemini {
		t.Fatalf("expected gemini (no claude available), got %#v", p)
	}
}

func TestRouter_Choose_CodingFallsBackIfNoClaudeNoGemini(t *testing.T) {
	r := &Router{
		Claude: nil,
		Gemini: nil,
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI},
	}

	p := r.Choose("I'm expecting to talk to OpenAI")
	if p == nil || p.Name() != llm.ProviderOpenAI {
		t.Fatalf("expected openai (no claude & gemini available), got %#v", p)
	}
}
