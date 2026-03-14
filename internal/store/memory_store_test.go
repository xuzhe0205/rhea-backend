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

	// 4. 验证乐观锁：如果传入错误的 parentID，应该报错（模拟并发冲突）
	wrongID := "some-random-id"
	_, err = s.UpdateConversationStatus(ctx, convID, uuid.NewString(), &wrongID, 50)
	if err == nil || err.Error() != "concurrent_conflict" {
		t.Errorf("Expected concurrent_conflict error, got %v", err)
	}

	// 5. 验证最终存储的状态
	conv, _ := s.GetConversation(ctx, convID)
	if conv.CumulativeTokens != 250 {
		t.Errorf("Final CumulativeTokens error, got %d", conv.CumulativeTokens)
	}
	if conv.LastMsgID.String() != msgID2 {
		t.Errorf("Final LastMsgID error, got %s", conv.LastMsgID)
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
	conv := "c1"

	msgs := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
		{Role: model.RoleUser, Content: "3"},
		{Role: model.RoleUser, Content: "4"},
	}

	for _, m := range msgs {
		_, _ = s.AppendMessage(ctx, conv, nil, m, nil)
	}

	// 注意：你的 MemoryStore 实现里 DESC 是最新的在前面
	// 如果 limit 2，应该是 4 和 3
	got, err := s.GetMessagesByConvID(ctx, conv, 2, "desc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID error: %v", err)
	}
	// 根据你的 memory_store 实现逻辑，desc 且 limit 2 拿到的应该是最后插入的两条
	if len(got) != 2 || got[0].Content != "4" || got[1].Content != "3" {
		t.Fatalf("unexpected messages: %v", got)
	}
}

func TestMemoryStore_GetMessagesByConvID_all(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	conv := "c1"

	msgs := []model.Message{
		{Role: model.RoleUser, Content: "1"},
		{Role: model.RoleUser, Content: "2"},
	}

	for _, m := range msgs {
		_, _ = s.AppendMessage(ctx, conv, nil, m, nil)
	}

	got, err := s.GetMessagesByConvID(ctx, conv, 0, "asc", "")
	if err != nil {
		t.Fatalf("GetMessagesByConvID error: %v", err)
	}
	if len(got) != 2 || got[0].Content != "1" || got[1].Content != "2" {
		t.Fatalf("unexpected messages: %v", got)
	}
}
