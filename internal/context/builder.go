/*
Package context handles the construction of the final prompt payload
that will be sent to the LLM.
It does NOT:
- Call the LLM
- Store data
- Route to different models
- Handle HTTP
It only prepares the context window.
*/
package context

import (
	"context"
	"errors"
	"fmt"

	"rhea-backend/internal/model"
	"rhea-backend/internal/store"
)

type Builder struct {
	Store         store.Store
	SystemPrompt  string
	RecentMaxMsgs int
}

/*
A context includes:
1️⃣ System prompt
Defines personality and behavior.
Example: "You are Rhea, a helpful AI assistant."
2️⃣ Summary (optional)
Compressed long-term memory.
Prevents token explosion.
Example: "User is learning Go. Prefers detailed explanations."
3️⃣ Recent messages
Short-term memory.
The last N messages in the conversation.
Maintains conversational continuity.
4️⃣ New user message
The latest input that needs a response.

Ordering is deliberate.
LLMs process messages sequentially.
*/

func (b *Builder) Build(ctx context.Context, conversationID string, userMsg string) ([]model.Message, error) {
	// 保持你原有的 Start Log 风格
	fmt.Printf("\n--- [Builder] Starting context build for Conv: %s ---\n", conversationID)

	if b.SystemPrompt == "" {
		return nil, errors.New("system prompt is required")
	}

	// 1. 获取 Summary
	summary, err := b.Store.GetSummary(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if summary != "" {
		fmt.Printf("[Builder] Found Summary: %s...\n", truncate(summary, 30))
	}

	// 2. 获取历史消息
	// 修改点：传入 b.RecentMaxMsgs, order 为 "asc", beforeID 为 ""
	// 这样 DAO 会帮你处理：DESC 查询最新 -> Reverse 为 ASC
	recent, err := b.Store.GetMessagesByConvID(ctx, conversationID, b.RecentMaxMsgs, "asc", "")
	if err != nil {
		return nil, err
	}
	fmt.Printf("[Builder] Retrieved %d recent messages from DB (Order: ASC)\n", len(recent))

	// 预分配容量
	maxMsgs := 2 + len(recent) + 1
	msgs := make([]model.Message, 0, maxMsgs)

	// 1) System Prompt
	msgs = append(msgs, model.Message{
		Role:    model.RoleSystem,
		Content: b.SystemPrompt,
	})

	// 2) Optional rolling summary
	if summary != "" {
		msgs = append(msgs, model.Message{
			Role:    model.RoleSystem,
			Content: "Conversation summary so far:\n" + summary,
		})
	}

	// 3) Recent messages (现在 recent 已经是 [旧...新] 顺序了)
	msgs = append(msgs, recent...)

	// 4) Current user message
	if userMsg != "" {
		fmt.Printf("[Builder] Appending new user message: %s\n", truncate(userMsg, 50))
		msgs = append(msgs, model.Message{
			Role:    model.RoleUser,
			Content: userMsg,
		})
	}

	// --- 终极调试打印：展示最终发给 LLM 的完整序列 ---
	fmt.Println("--- [Builder] Final Message Sequence for LLM ---")
	for i, m := range msgs {
		// 增加了一个 ID 的打印，方便你和数据库对齐
		shortID := "system"
		if m.ID != [16]byte{} {
			shortID = m.ID.String()[:8]
		}
		fmt.Printf("  [%d] Role: %-10s | ID: %s | Content: %s\n", i, m.Role, shortID, truncate(m.Content, 80))
	}
	fmt.Println("...")
	fmt.Printf("msgs: %+v\n", msgs)
	return msgs, nil
}

// 辅助函数，防止 Log 过长刷屏
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
