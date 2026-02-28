package store

import (
	"context"
	"fmt"
	"testing"

	"rhea-backend/internal/model"
)

func TestMemoryStore_SummaryRoundTrip(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	conv := "c1"
	fmt.Println("yoyoyo: " + conv)

	got, err := s.GetSummary(ctx, conv)
	if err != nil {
		t.Fatalf("GetSummary error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}

	if err := s.SetSummary(ctx, conv, "hello"); err != nil {
		t.Fatalf("SetSummary error: %v", err)
	}

	got, err = s.GetSummary(ctx, conv)
	if err != nil {
		t.Fatalf("GetSummary error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}

func TestMemoryStore_GetMessagesByConvID(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	conv := "c1"

	msgs := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
		{Role: model.RoleUser, Content: "3"},
		{Role: model.RoleUser, Content: "4"},
	}

	for _, m := range msgs {
		if _, err := s.AppendMessage(ctx, conv, nil, m, nil); err != nil {
			t.Fatalf("AppendMessage error: %v", err)
		}
	}

	got, err := s.GetMessagesByConvID(ctx, conv, 2, "desc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID error: %v", err)
	}
	if len(got) != 2 || got[0].Content != "3" || got[1].Content != "4" {
		t.Fatalf("unexpected recent messages: %#v", got)
	}
}

func TestMemoryStore_GetMessagesByConvID_all(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	conv := "c1"

	msgs := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
		{Role: model.RoleUser, Content: "3"},
		{Role: model.RoleUser, Content: "4"},
	}

	for _, m := range msgs {
		if _, err := s.AppendMessage(ctx, conv, nil, m, nil); err != nil {
			t.Fatalf("AppendMessage error: %v", err)
		}
	}

	got, err := s.GetMessagesByConvID(ctx, conv, 0, "desc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID error: %v", err)
	}
	if len(got) != 4 || got[0].Content != "1" || got[1].Content != "2" || got[2].Content != "3" || got[3].Content != "4" {
		t.Fatalf("unexpected recent messages: %#v", got)
	}
}

func TestMemoryStore_GetMessagesByConvID_multiSession(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	conv1 := "c1"
	conv2 := "c2"

	msgsC1 := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
		{Role: model.RoleUser, Content: "3"},
		{Role: model.RoleUser, Content: "4"},
	}

	msgsC2 := []model.Message{
		{Role: model.RoleUser, Content: "5"},
	}

	for _, m := range msgsC1 {
		if _, err := s.AppendMessage(ctx, conv1, nil, m, nil); err != nil {
			t.Fatalf("AppendMessage error: %v", err)
		}
	}

	for _, m := range msgsC2 {
		if _, err := s.AppendMessage(ctx, conv2, nil, m, nil); err != nil {
			t.Fatalf("AppendMessage error: %v", err)
		}
	}

	gotC1, errC1 := s.GetMessagesByConvID(ctx, conv1, 0, "desc", "")
	if errC1 != nil {
		t.Fatalf("GetMessagesByConvID error: %v", errC1)
	}
	for _, m := range gotC1 {
		if m.Content == "5" {
			t.Fatalf("conv1 messages should not contain conv2 content: %#v", gotC1)
		}
	}

	gotC2, errC2 := s.GetMessagesByConvID(ctx, conv2, 0, "desc", "")
	if errC2 != nil {
		t.Fatalf("GetMessagesByConvID error: %v", errC2)
	}
	if len(gotC2) != 1 || gotC2[0].Content != "5" {
		t.Fatalf("unexpected recent messages: %#v", gotC2)
	}
}
