package llm

import (
	"testing"

	"google.golang.org/genai"

	"rhea-backend/internal/model"
)

func TestToGenAIContents_RolesAndOrderPreserved(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleSystem, Content: "You are Rhea."},
		{Role: model.RoleUser, Content: "hi"},
		{Role: model.RoleAssistant, Content: "hello!"},
		{Role: model.RoleUser, Content: "how are you?"},
	}

	got := toGenAIContents(msgs)

	if len(got) != len(msgs) {
		t.Fatalf("expected %d contents, got %d", len(msgs), len(got))
	}

	// 1) Order preserved + content preserved
	for i := range msgs {
		if got[i] == nil {
			t.Fatalf("got[%d] is nil", i)
		}
		if len(got[i].Parts) != 1 || got[i].Parts[0] == nil {
			t.Fatalf("expected exactly 1 text part at index %d, got %#v", i, got[i].Parts)
		}
		if got[i].Parts[0].Text != msgs[i].Content {
			t.Fatalf("content mismatch at index %d: expected %q, got %q", i, msgs[i].Content, got[i].Parts[0].Text)
		}
	}

	// 2) Role mapping
	// Current v1 mapping in your helper:
	// - assistant -> genai.RoleModel
	// - user -> genai.RoleUser
	// - system -> genai.RoleUser (simple v1 choice)
	if got[0].Role != genai.RoleUser {
		t.Fatalf("system role should map to RoleUser (v1), got %q", got[0].Role)
	}
	if got[1].Role != genai.RoleUser {
		t.Fatalf("user role should map to RoleUser, got %q", got[1].Role)
	}
	if got[2].Role != genai.RoleModel {
		t.Fatalf("assistant role should map to RoleModel, got %q", got[2].Role)
	}
	if got[3].Role != genai.RoleUser {
		t.Fatalf("user role should map to RoleUser, got %q", got[3].Role)
	}

	// 3) System prompt ends up first (builder behavior assumption)
	if got[0].Parts[0].Text != "You are Rhea." {
		t.Fatalf("expected system prompt first, got %q", got[0].Parts[0].Text)
	}
}
