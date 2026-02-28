package agent

import (
	"context"
	"errors"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"
	"testing"
)

func TestServiceStream_PersistErrorOnStreamError(t *testing.T) {
	ctx := context.Background()
	conversationID := "test"
	userText := "hello world"
	emit := func(delta string) error {
		return nil
	}
	st := store.NewMemoryStore()
	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  "You are Rhea.",
		RecentMaxMsgs: 10,
	}
	fpGemini := &llm.FakeProvider{Provider: llm.ProviderClaude, Reply: "assistant says hi", Err: errors.New("stream failed")}

	r := &router.Router{
		Gemini: fpGemini,
	}

	service := &Service{st, b, r}

	conversationID, err := service.ChatStream(ctx, conversationID, userText, emit)

	if errors.Is(err, errors.New("stream failed")) {
		t.Fatalf("expecting error 'stream failed', but got '%s'", err)
	}

	persistedMsg, error := st.GetMessagesByConvID(ctx, conversationID, 0, "desc", "")
	if error != nil {
		t.Fatalf("Not expecting error")
	}
	if persistedMsg[0].Content != userText {
		t.Fatalf("Expecting '%s' persisted in the Store", userText)
	}
}
