package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"rhea-backend/internal/auth"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

func TestServiceStream_FailoverAndTokenAccounting(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

	pFreeFail := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Model:        "flash-free-v3",
		Err:          fmt.Errorf("429 RESOURCE_EXHAUSTED: quota exceeded"),
	}

	pPaidSuccess := &llm.FakeProvider{
		ProviderName: llm.ProviderGemini,
		Model:        "flash-paid-v3",
		Reply:        "I am the paid backup, and I work!",
		Usage: &llm.Usage{
			InputTokens:  40,
			OutputTokens: 60,
			ModelName:    "flash-paid-v3",
		},
	}

	r := &router.Router{
		GeminiLite: &llm.FakeProvider{
			Reply: "SIMPLE",
			Usage: &llm.Usage{InputTokens: 0, OutputTokens: 0},
		},
		GeminiFlashFree: pFreeFail,
		GeminiFlash:     pPaidSuccess,
		GeminiPro:       &llm.FakeProvider{Model: "pro-v2"},
	}

	svc := &Service{
		Store:   st,
		Builder: &ctxbuilder.Builder{Store: st, SystemPrompt: "Hi"},
		Router:  r,
	}

	var emittedParts []string
	var metaEvents []map[string]any

	cb := StreamCallbacks{
		OnDelta: func(delta string) error {
			emittedParts = append(emittedParts, delta)
			return nil
		},
		OnMeta: func(payload map[string]any) error {
			// copy 一份，避免后续引用同一 map
			cp := make(map[string]any, len(payload))
			for k, v := range payload {
				cp[k] = v
			}
			metaEvents = append(metaEvents, cp)
			return nil
		},
	}

	convID, err := svc.ChatStream(ctx, "", "Can you hear me?", cb)
	if err != nil {
		t.Fatalf("ChatStream failed despite failover: %v", err)
	}

	fullResponse := strings.Join(emittedParts, "")
	if fullResponse != "I am the paid backup, and I work!" {
		t.Errorf("Unexpected response content: %s", fullResponse)
	}

	if len(metaEvents) < 2 {
		t.Fatalf("expected at least 2 meta events, got %d", len(metaEvents))
	}

	// 第一段 meta：conversation_id + user_message_id
	firstMeta := metaEvents[0]
	if firstMeta["conversation_id"] != convID {
		t.Errorf("first meta conversation_id mismatch: expected %s, got %v", convID, firstMeta["conversation_id"])
	}
	if _, ok := firstMeta["user_message_id"].(string); !ok {
		t.Errorf("first meta missing user_message_id: %+v", firstMeta)
	}

	// 第二段 meta：conversation_id + assistant_message_id (+ optional title)
	foundAssistantMeta := false
	for _, m := range metaEvents {
		if m["conversation_id"] == convID {
			if _, ok := m["assistant_message_id"].(string); ok {
				foundAssistantMeta = true
				break
			}
		}
	}
	if !foundAssistantMeta {
		t.Error("assistant_message_id metadata was not emitted")
	}

	conv, err := st.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}

	expectedMainReplyTokens := 40 + 60
	expectedTitleTokens := 100
	expectedTotal := expectedMainReplyTokens + expectedTitleTokens

	if conv.CumulativeTokens != expectedTotal {
		t.Errorf("Token accounting failed: expected %d, got %d", expectedTotal, conv.CumulativeTokens)
	}

	msgs, err := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID failed: %v", err)
	}

	var assistantMsg *model.Message
	for _, m := range msgs {
		if m.Role == model.RoleAssistant {
			msgCopy := m
			assistantMsg = &msgCopy
			break
		}
	}

	if assistantMsg == nil {
		t.Fatal("Assistant message was not persisted")
	}
	if assistantMsg.InputTokens != 40 || assistantMsg.OutputTokens != 60 {
		t.Errorf(
			"Message-level tokens error: got %d/%d, expected 40/60",
			assistantMsg.InputTokens,
			assistantMsg.OutputTokens,
		)
	}
}

func TestServiceStream_UserMessagePersistence(t *testing.T) {
	uid := uuid.New()
	ctx := auth.SetUserID(context.Background(), uid)
	st := store.NewMemoryStore()

	pFatal := &llm.FakeProvider{Err: fmt.Errorf("network unreachable")}
	r := &router.Router{
		GeminiLite:      &llm.FakeProvider{Reply: "DEEP", Usage: &llm.Usage{}},
		GeminiPro:       pFatal,
		GeminiFlashFree: pFatal,
		GeminiFlash:     pFatal,
	}

	svc := &Service{
		Store:   st,
		Builder: &ctxbuilder.Builder{Store: st, SystemPrompt: "Hi"},
		Router:  r,
	}

	cb := StreamCallbacks{
		OnDelta: func(string) error { return nil },
		OnMeta:  func(map[string]any) error { return nil },
	}

	convID, err := svc.ChatStream(ctx, "", "Persistence test", cb)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	msgs, err := st.GetMessagesByConvID(ctx, convID, 10, "asc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID failed: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != model.RoleUser {
		t.Fatalf("User message should be persisted even if AI fails, got msgs=%+v", msgs)
	}

	conv, err := st.GetConversation(ctx, convID)
	if err != nil || conv == nil {
		t.Fatalf("conversation should exist even on AI failure, err=%v conv=%v", err, conv)
	}
	if conv.CumulativeTokens != 0 {
		t.Errorf("Tokens should be 0 on total failure, got %d", conv.CumulativeTokens)
	}
}
