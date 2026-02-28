package agent

import (
	"fmt"
	"time"
)

const (
	TitleGeneratorPrompt = "You are a helpful assistant that creates short, concise titles for chat conversations. " +
		"Summarize the user's first message in 5-8 words in the same language as the message."
)

// GetDefaultSystemPrompt 动态生成包含时间锚点的系统指令
func GetDefaultSystemPrompt() string {
	now := time.Now().Format("Monday, January 02, 2006")

	// 采用更强硬的语气，防止 Gemini 产生 2024 年的幻觉
	return fmt.Sprintf(
		"You are Rhea, a helpful AI assistant. \n"+
			"CRITICAL CONTEXT: The current real-world date is %s. \n"+
			"If your internal data suggests a different date (like July 2024), it is INCORRECT. "+
			"Always prioritize this provided date for all calculations and responses.",
		now,
	)
}
