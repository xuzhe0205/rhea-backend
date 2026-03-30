package store

import (
	"context"
	"testing"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
)

// --- 原有的测试保持并微调 ---

func TestMemoryStore_SummaryRoundTrip(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	conv := "c1"

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

// --- 🚀 新增：测试 Token 原子累加和状态更新 ---

func TestMemoryStore_UpdateConversationStatus_Atomic(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	uid := uuid.New()
	convID := uuid.New().String()

	// 1. 创建对话
	_, _ = s.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID), UserID: uid})

	// 2. 第一次更新：模拟第一条 AI 回复
	msgID1 := uuid.NewString()
	// 初始状态 lastMsgID 为空，我们传入 nil
	total, err := s.UpdateConversationStatus(ctx, convID, msgID1, nil, 100)
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}
	if total != 100 {
		t.Errorf("Expected total 100, got %d", total)
	}

	// 3. 第二次更新：模拟第二条 AI 回复 (累加 150)
	msgID2 := uuid.NewString()
	total, err = s.UpdateConversationStatus(ctx, convID, msgID2, &msgID1, 150)
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}
	if total != 250 {
		t.Errorf("Expected total 250 (100+150), got %d", total)
	}

	// 4. 现在不再做乐观锁冲突校验；即使 oldLastMsgID 不匹配，也会直接更新
	wrongID := "some-random-id"
	msgID3 := uuid.NewString()
	total, err = s.UpdateConversationStatus(ctx, convID, msgID3, &wrongID, 50)
	if err != nil {
		t.Errorf("Expected no error on mismatched oldLastMsgID, got %v", err)
	}
	if total != 300 {
		t.Errorf("Expected total 300 (100+150+50), got %d", total)
	}

	// 5. 验证最终存储的状态
	conv, err := s.GetConversation(ctx, convID)
	if err != nil || conv == nil {
		t.Fatalf("GetConversation failed: err=%v conv=%v", err, conv)
	}
	if conv.CumulativeTokens != 300 {
		t.Errorf("Final CumulativeTokens error, got %d", conv.CumulativeTokens)
	}
	if conv.LastMsgID == nil || conv.LastMsgID.String() != msgID3 {
		t.Errorf("Final LastMsgID error, got %v", conv.LastMsgID)
	}
}

func TestMemoryStore_IncrementTokenUsage(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	convID := uuid.New().String()
	_, _ = s.CreateConversation(ctx, &model.Conversation{ID: uuid.MustParse(convID)})

	// 模拟异步任务（如 TitleGen）多次增加 Token
	_ = s.IncrementConversationTokenUsage(ctx, convID, 10)
	_ = s.IncrementConversationTokenUsage(ctx, convID, 20)

	conv, _ := s.GetConversation(ctx, convID)
	if conv.CumulativeTokens != 30 {
		t.Errorf("Expected 30 tokens, got %d", conv.CumulativeTokens)
	}
}

// --- 消息查询测试保持逻辑，修正数据校验 ---

func TestMemoryStore_GetMessagesByConvID(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	convID := uuid.New()
	conv := convID.String()
	_, err := s.CreateConversation(ctx, &model.Conversation{
		ID:     convID,
		UserID: uuid.New(),
		Title:  "test",
	})
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	msgs := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
		{Role: model.RoleUser, Content: "3"},
		{Role: model.RoleUser, Content: "4"},
	}

	for _, m := range msgs {
		_, err := s.AppendMessage(ctx, conv, nil, m, nil)
		if err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	got, err := s.GetMessagesByConvID(ctx, conv, 2, "desc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID error: %v", err)
	}
	if len(got) != 2 || got[0].Content != "4" || got[1].Content != "3" {
		t.Fatalf("unexpected messages: %v", got)
	}
}

func TestMemoryStore_GetMessagesByConvID_all(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	convID := uuid.New()
	conv := convID.String()
	_, err := s.CreateConversation(ctx, &model.Conversation{
		ID:     convID,
		UserID: uuid.New(),
		Title:  "test",
	})
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	msgs := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
	}

	for _, m := range msgs {
		_, err := s.AppendMessage(ctx, conv, nil, m, nil)
		if err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	got, err := s.GetMessagesByConvID(ctx, conv, 0, "asc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID error: %v", err)
	}
	if len(got) != 2 || got[0].Content != "1" || got[1].Content != "2" {
		t.Fatalf("unexpected messages: %v", got)
	}
}
