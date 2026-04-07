package context

import (
	"context"
	"strings"
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

	result, err := b.Build(ctx, BuildInput{
		ConversationID: conv,
		UserMsg:        "new msg",
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Expected order:
	// [0] system prompt
	// [1] recent #1 (assistant hello)
	// [2] recent #2 (user how are you?)
	// [3] new user message
	if len(result.Messages) !=4 {
		t.Fatalf("expected 4 messages, got %d: %#v", len(result.Messages), result.Messages)
	}

	if result.Messages[0].Role != model.RoleSystem || result.Messages[0].Content != "You are Rhea." {
		t.Fatalf("unexpected system prompt: %#v", result.Messages[0])
	}

	if result.Messages[1].Content != "hello" || result.Messages[2].Content != "how are you?" {
		t.Fatalf("unexpected recent messages: %#v", result.Messages[1:3])
	}

	if result.Messages[3].Role != model.RoleUser || result.Messages[3].Content != "new msg" {
		t.Fatalf("unexpected new user msg: %#v", result.Messages[3])
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

	result, err := b.Build(ctx, BuildInput{
		ConversationID: conv,
		UserMsg:        "new msg",
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Expected order:
	// [0] system prompt + embedded summary
	// [1] recent old1
	// [2] new user message
	if len(result.Messages) !=3 {
		t.Fatalf("expected 3 messages, got %d: %#v", len(result.Messages), result.Messages)
	}

	if result.Messages[0].Role != model.RoleSystem {
		t.Fatalf("expected first message to be system, got %#v", result.Messages[0])
	}
	if !strings.Contains(result.Messages[0].Content, "You are Rhea.") {
		t.Fatalf("expected system prompt base text, got: %q", result.Messages[0].Content)
	}
	if !strings.Contains(result.Messages[0].Content, "Conversation summary so far:") {
		t.Fatalf("expected summary wrapper in system prompt, got: %q", result.Messages[0].Content)
	}
	if !strings.Contains(result.Messages[0].Content, "User is learning Go. Prefers SSE streaming.") {
		t.Fatalf("expected summary content in system prompt, got: %q", result.Messages[0].Content)
	}

	if result.Messages[1].Content != "old1" {
		t.Fatalf("unexpected recent message: %#v", result.Messages[1])
	}
	if result.Messages[2].Role != model.RoleUser || result.Messages[2].Content != "new msg" {
		t.Fatalf("unexpected new user message: %#v", result.Messages[2])
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
		RecentMaxMsgs: 0, // No limit on recent messages
	}

	result, err := b.Build(ctx, BuildInput{
		ConversationID: conv,
		UserMsg:        "new msg",
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Expected order:
	// [0] system prompt
	// [1] recent #1 (user hi)
	// [2] recent #2 (assistant hello)
	// [3] recent #3 (user how are you?)
	// [4] new user message
	if len(result.Messages) !=5 {
		t.Fatalf("expected 5 messages, got %d: %#v", len(result.Messages), result.Messages)
	}

	if result.Messages[0].Role != model.RoleSystem || result.Messages[0].Content != "You are Rhea." {
		t.Fatalf("unexpected system prompt: %#v", result.Messages[0])
	}

	if result.Messages[1].Content != "hi" || result.Messages[2].Content != "hello" || result.Messages[3].Content != "how are you?" {
		t.Fatalf("unexpected recent messages: %#v", result.Messages[1:4])
	}

	if result.Messages[4].Role != model.RoleUser || result.Messages[4].Content != "new msg" {
		t.Fatalf("unexpected new user msg: %#v", result.Messages[4])
	}
}

func TestBuilder_Build_NoSystemPrompt(t *testing.T) {
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
		RecentMaxMsgs: 2,
	}

	_, err = b.Build(ctx, BuildInput{
		ConversationID: conv,
		UserMsg:        "new msg",
	})
	if err == nil {
		t.Fatalf("expected build error")
	}

	expected := "system prompt is required"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}
