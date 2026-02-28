package router

import (
	"strings"

	"rhea-backend/internal/llm"
)

type Router struct {
	Claude llm.Provider
	Gemini llm.Provider
	OpenAI llm.Provider
}

// Choose picks a provider based on a v0 heuristic.
// v0 rule: if looks like coding -> Claude, else -> Gemini, fallback -> OpenAI
func (r *Router) Choose(userText string) llm.Provider {
	if isCoding(userText) && r.Claude != nil {
		return r.Claude
	}
	if r.Gemini != nil {
		return r.Gemini
	}
	return r.OpenAI
}

func (r *Router) ChooseChain(userText string) []llm.Provider {
	var providerPriorityChain []llm.Provider
	if isCoding(userText) && r.Claude != nil {
		providerPriorityChain = append(providerPriorityChain, r.Claude, r.Gemini, r.OpenAI)
	} else if isGeminiUsageOverThreshold() {
		providerPriorityChain = append(providerPriorityChain, r.Gemini, r.OpenAI, r.Claude)
	} else {
		providerPriorityChain = append(providerPriorityChain, r.OpenAI, r.Gemini, r.Claude)
	}
	return providerPriorityChain
}

func isGeminiUsageOverThreshold() bool {
	// TODO: check token usage for the given Gemini model, if token >= free tier threshold, switch to fallback LLM.
	// If all LLMs are exceeding their free-tier threshold, just use Gemini. For now just always true.
	// resp, err := client.Models.GenerateContent(ctx, modelName, contents, nil) --> usage := resp.UsageMetadata --> ...
	return true
}

func isCoding(s string) bool {
	s = strings.ToLower(s)

	// Cheap heuristics. We'll improve later.
	if strings.Contains(s, "```") {
		return true
	}
	if strings.Contains(s, "error") || strings.Contains(s, "stack trace") {
		return true
	}
	if strings.Contains(s, "golang") || strings.Contains(s, "java") || strings.Contains(s, "python") {
		return true
	}
	if strings.Contains(s, "compile") || strings.Contains(s, "build failed") {
		return true
	}
	if strings.Contains(s, "func ") || strings.Contains(s, "class ") {
		return true
	}
	// if IsCodingIntent(s) {
	// 	return true
	// }
	return false
}

func IsCodingIntent(query string) bool {
	query = strings.ToLower(query)

	hasAction := containsAny(query, "script", "code", "debug", "implement", "function")
	hasTech := containsAny(query, "s3", "api", "json", "database", "python", "java")

	// If it has a coding action OR mentions a specific tech stack, route it.
	return hasAction || hasTech
}

// Helper to keep the main logic clean
func containsAny(s string, keywords ...string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
