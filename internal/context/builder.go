package context

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"rhea-backend/internal/model"
	"rhea-backend/internal/rag"
	"rhea-backend/internal/retrieval"
	"rhea-backend/internal/store"
)

type Builder struct {
	Store         store.Store
	Retrieval     *retrieval.Service
	SystemPrompt  string
	RecentMaxMsgs int
	RetrievalTopK int
}

type BuildInput struct {
	ConversationID string
	UserMsg        string
}

func (b *Builder) Build(ctx context.Context, in BuildInput) ([]model.Message, error) {
	fmt.Printf("\n--- [Builder] Starting context build for Conv: %s ---\n", in.ConversationID)

	if b.SystemPrompt == "" {
		return nil, errors.New("system prompt is required")
	}

	conv, err := b.Store.GetConversation(ctx, in.ConversationID)
	if err != nil {
		return nil, err
	}

	recent, err := b.Store.GetMessagesByConvID(ctx, in.ConversationID, b.RecentMaxMsgs, "asc", "")
	if err != nil {
		return nil, err
	}

	systemText := buildSystemPrompt(b.SystemPrompt, conv.Summary)

	msgs := make([]model.Message, 0, 2+len(recent)+1)
	msgs = append(msgs, model.Message{
		Role:    model.RoleSystem,
		Content: systemText,
	})

	if b.Retrieval != nil {
		scope := rag.ScopeConversationOnly
		if conv.ProjectID != nil {
			scope = rag.ScopeConversationAndProject
		}

		rc, err := b.Retrieval.Retrieve(ctx, retrieval.QueryInput{
			UserID:         conv.UserID,
			ConversationID: conv.ID,
			ProjectID:      conv.ProjectID,
			Query:          in.UserMsg,
			TopK:           b.RetrievalTopK,
			Scope:          scope,
		})
		if err != nil {
			return nil, err
		}

		if rc != nil && len(rc.Chunks) > 0 {
			log.Printf("[Builder] conv=%s retrieved_chunks=%d recent_msgs=%d",
				in.ConversationID, len(rc.Chunks), len(recent))

			for i, ch := range rc.Chunks {
				if i >= 3 {
					break
				}
				log.Printf(
					"[Builder] retrieved[%d] score=%.4f vector=%.4f keyword=%.4f source=%s chunk=%d preview=%q",
					i,
					ch.FinalScore,
					ch.VectorScore,
					ch.KeywordScore,
					ch.Chunk.SourceType,
					ch.Chunk.ChunkIndex,
					truncateForLog(ch.Chunk.Content, 120),
				)
			}

			msgs = append(msgs, model.Message{
				Role:    model.RoleUser,
				Content: formatRetrievedContext(rc),
			})
		}
	}

	msgs = append(msgs, recent...)

	msgs = append(msgs, model.Message{
		Role:    model.RoleUser,
		Content: in.UserMsg,
	})

	return msgs, nil
}

func buildSystemPrompt(base string, summary string) string {
	base = strings.TrimSpace(base)
	summary = strings.TrimSpace(summary)

	if summary == "" {
		return base
	}

	return base + "\n\nConversation summary so far:\n" + summary
}

func formatRetrievedContext(rc *retrieval.RetrievedContext) string {
	if rc == nil || len(rc.Chunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== RAG_MEMORY_CONTEXT ===\n")
	sb.WriteString("Use the following retrieved memory only if it is relevant to the user's latest question.\n")
	sb.WriteString("If some retrieved memory is not relevant, ignore it.\n\n")

	for i, ch := range rc.Chunks {
		sb.WriteString(fmt.Sprintf(
			"[Memory %d]\nSourceType: %s\nContent:\n%s\n\n",
			i+1,
			ch.Chunk.SourceType,
			strings.TrimSpace(ch.Chunk.Content),
		))
	}

	return sb.String()
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
