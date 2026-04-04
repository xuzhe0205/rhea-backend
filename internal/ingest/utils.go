package ingest

import (
	"rhea-backend/internal/model"
	"strings"
)

func buildConversationRawText(msgs []model.Message) string {
	var sb strings.Builder

	for _, m := range msgs {
		role := strings.ToUpper(string(m.Role))
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(strings.TrimSpace(m.Content))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func chunkText(s string, targetSize int, overlap int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if targetSize <= 0 {
		targetSize = 1200
	}
	if overlap < 0 {
		overlap = 0
	}

	runes := []rune(s)
	var out []string

	start := 0
	for start < len(runes) {
		end := start + targetSize
		if end > len(runes) {
			end = len(runes)
		}

		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			out = append(out, chunk)
		}

		if end == len(runes) {
			break
		}

		start = end - overlap
		if start < 0 {
			start = 0
		}
	}

	return out
}
