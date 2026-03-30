package context

import (
	"context"
	"testing"

	"rhea-backend/internal/model"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

func TestBuilder_Build_NoSummary(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore()

	convID := uuid.New()
	conv := convID.String()

	_, err := s.CreateConversation(ctx, &model.Conversation{
		ID:     convID,
		UserID: uuid.New(),
		Title:  "test",
	})
	if err != nil {
		t.Fatalf("CreateConversation error: %v", err)
	}

	// Seed some history
	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "hi"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}
	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleAssistant, Content: "hello"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}
	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "how are you?"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}

	b := &Builder{
		Store:         s,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 2, // only last 2 should be included
	}

	got, err := b.Build(ctx, conv, "new msg")
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Expected order:
	// [0] system prompt
	// [1] recent #1 (assistant hello)  (because last 2)
	// [2] recent #2 (user how are you?)
	// [3] new user message
	if len(got) != 4 {
		t.Fatalf("expected 4 messages, got %d: %#v", len(got), got)
	}

	if got[0].Role != model.RoleSystem || got[0].Content != "You are Rhea." {
		t.Fatalf("unexpected system prompt: %#v", got[0])
	}

	if got[1].Content != "hello" || got[2].Content != "how are you?" {
		t.Fatalf("unexpected recent messages: %#v", got[1:3])
	}

	if got[3].Role != model.RoleUser || got[3].Content != "new msg" {
		t.Fatalf("unexpected new user msg: %#v", got[3])
	}
}

func TestBuilder_Build_WithSummary(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore()

	convID := uuid.New()
	conv := convID.String()

	_, err := s.CreateConversation(ctx, &model.Conversation{
		ID:     convID,
		UserID: uuid.New(),
		Title:  "test",
	})
	if err != nil {
		t.Fatalf("CreateConversation error: %v", err)
	}

	err = s.SetSummary(ctx, conv, "User is learning Go. Prefers SSE streaming.")
	if err != nil {
		t.Fatalf("SetSummary error: %v", err)
	}

	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "old1"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}

	b := &Builder{
		Store:         s,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}

	got, err := b.Build(ctx, conv, "new msg")
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Expected order:
	// [0] system prompt
	// [1] summary system message
	// [2] recent old1
	// [3] new user message
	if len(got) != 4 {
		t.Fatalf("expected 4 messages, got %d: %#v", len(got), got)
	}

	if got[1].Role != model.RoleSystem {
		t.Fatalf("expected summary to be RoleSystem, got %#v", got[1])
	}
	if got[1].Content == "" || got[1].Content == "User is learning Go. Prefers SSE streaming." {
		t.Fatalf("expected summary to be wrapped with prefix, got: %q", got[1].Content)
	}

	if got[2].Content != "old1" {
		t.Fatalf("unexpected recent message: %#v", got[2])
	}
	if got[3].Content != "new msg" {
		t.Fatalf("unexpected new user message: %#v", got[3])
	}
}

func TestBuilder_Build_NoRecentMaxMsgs(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore()

	convID := uuid.New()
	conv := convID.String()

	_, err := s.CreateConversation(ctx, &model.Conversation{
		ID:     convID,
		UserID: uuid.New(),
		Title:  "test",
	})
	if err != nil {
		t.Fatalf("CreateConversation error: %v", err)
	}

	// Seed some history
	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "hi"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}
	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleAssistant, Content: "hello"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}
	_, err = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "how are you?"}, nil)
	if err != nil {
		t.Fatalf("AppendMessage error: %v", err)
	}

	b := &Builder{
		Store:         s,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 0, // No limit on recent message
	}

	got, err := b.Build(ctx, conv, "new msg")
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Expected order:
	// [0] system prompt
	// [1] recent #1 (user hi)
	// [2] recent #2 (assistant hello)
	// [3] recent #3 (user how are you?)
	// [4] new user message
	if len(got) != 5 {
		t.Fatalf("expected 5 messages, got %d: %#v", len(got), got)
	}

	if got[0].Role != model.RoleSystem || got[0].Content != "You are Rhea." {
		t.Fatalf("unexpected system prompt: %#v", got[0])
	}

	if got[1].Content != "hi" || got[2].Content != "hello" || got[3].Content != "how are you?" {
		t.Fatalf("unexpected recent messages: %#v", got[1:4])
	}

	if got[4].Role != model.RoleUser || got[4].Content != "new msg" {
		t.Fatalf("unexpected new user msg: %#v", got[4])
	}
}

func TestBuilder_Build_NoSystemPrompt(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore()

	conv := "c1"
	// Seed some history
	_, _ = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "hi"}, nil)
	_, _ = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleAssistant, Content: "hello"}, nil)
	_, _ = s.AppendMessage(ctx, conv, nil, model.Message{Role: model.RoleUser, Content: "how are you?"}, nil)

	b := &Builder{
		Store:         s,
		RecentMaxMsgs: 2, // only last 2 should be included
	}

	_, err := b.Build(ctx, conv, "new msg")
	if err == nil {
		t.Fatalf("Expecting build error...")
	}

	expected := "system prompt is required"
	if err.Error() != expected {
		t.Errorf("Expected error %q, but got %q", expected, err.Error())
	}

}
