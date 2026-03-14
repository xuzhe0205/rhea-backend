package router

import (
	"context"
	"fmt"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"strings"
)

type Router struct {
	GeminiPro   llm.Provider // 2.5 Pro (深度学习/重构)
	GeminiFlash llm.Provider // 3.0 Flash (日常问答)
	GeminiLite  llm.Provider // 3.1 Flash-Lite (极速分类)
}

const (
	IntentSimple = "SIMPLE" // 对应 Flash
	IntentDeep   = "DEEP"   // 对应 Pro
)

// IntentPrompt 极其精简，确保 Lite 响应在 300ms 内
const IntentPrompt = `Task: Determine if the query requires HIGH-REASONING (DEEP) or STANDARD-RESPONSE (SIMPLE).

- SIMPLE: Routine inquiries, factual retrieval, creative drafting, instruction following, or general explanations. This includes long processes like recipes or history if they don't require logical troubleshooting.
- DEEP: Requires multi-layered logical deduction, architectural system design, resolving conflicting constraints, or high-stakes professional auditing.

Strategy: 
1. If the request is a direct question about "how to do X" or "what is Y", classify as SIMPLE.
2. If the request is "analyze/debug/optimize X based on Y", classify as DEEP.
3. IF IN DOUBT, CHOOSE SIMPLE.

Output ONLY 'SIMPLE' or 'DEEP'.`

func (r *Router) ChooseChain(ctx context.Context, userText string) []llm.Provider {
	// 1. 强特征抢跑：如果有明显的代码块，直接上 Pro，省去一次意图检查
	if strings.Contains(userText, "```") || strings.Contains(userText, "func ") {
		return []llm.Provider{r.GeminiPro, r.GeminiFlash}
	}

	// 2. 调用极速分类器 (Lite)
	intent := r.classifyIntent(ctx, userText)
	fmt.Printf("[Router] Classified Intent: %s\n", intent)

	if intent == IntentSimple {
		// 简单问题：Flash 优先，Pro 保底
		return []llm.Provider{r.GeminiFlash, r.GeminiPro}
	}

	// 深度问题：Pro 优先，Flash 保底
	return []llm.Provider{r.GeminiPro, r.GeminiFlash}
}

func (r *Router) classifyIntent(ctx context.Context, text string) string {
	if r.GeminiLite == nil {
		return IntentDeep
	}

	msgs := []model.Message{
		{Role: model.RoleSystem, Content: IntentPrompt},
		{Role: model.RoleUser, Content: text},
	}

	// 使用非流式调用，只拿一个词
	resp, err := r.GeminiLite.Chat(ctx, msgs)
	if err != nil {
		return IntentDeep // 出错保底用 Pro
	}

	// 🚀 关键步骤：在这里你可以打印或记录这个分类动作花掉的 Token
	// 以后我们可以把这个 resp.Usage 传给一个专门的异步方法存入数据库
	fmt.Printf("[Router] Classification Usage - Model: %s, In: %d, Out: %d\n",
		resp.Usage.ModelName, resp.Usage.InputTokens, resp.Usage.OutputTokens)

	// 2. 从结构体中提取文本内容进行处理
	content := strings.TrimSpace(strings.ToUpper(resp.Content))

	if strings.Contains(content, IntentSimple) {
		return IntentSimple
	}
	return IntentDeep
}

// 保留原有的 Choose 逻辑用于内部任务（如标题生成）
func (r *Router) Choose(taskType string) llm.Provider {
	if taskType == "internal_task" {
		return r.GeminiFlash // 标题生成用 Flash 足够快且稳
	}
	return r.GeminiFlash
}
