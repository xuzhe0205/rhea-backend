package retrieval

import (
	"context"
	"fmt"
	"log"
	"strings"

	"rhea-backend/internal/embedding"
	"rhea-backend/internal/store"
)

type Service struct {
	Store      store.Store
	Embeddings *embedding.Service
	Policy     Policy
}

func (s *Service) Retrieve(ctx context.Context, in QueryInput) (*RetrievedContext, error) {
	if s == nil {
		return nil, fmt.Errorf("retrieval service is nil")
	}
	if s.Store == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if s.Embeddings == nil {
		return nil, fmt.Errorf("embedding service is nil")
	}

	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return &RetrievedContext{Chunks: nil}, nil
	}

	policy := s.effectivePolicy(in.TopK)
	rewrittenQuery := rewriteQuery(in.Query)

	queryEmbedding, err := s.Embeddings.EmbedText(ctx, rewrittenQuery)
	if err != nil {
		return nil, err
	}

	candidateK := policy.TopK * policy.CandidateMultiplier
	if candidateK < policy.TopK {
		candidateK = policy.TopK
	}

	vectorHits, err := s.Store.VectorSearchMemoryChunks(
		ctx,
		in.UserID,
		in.ConversationID,
		in.ProjectID,
		in.Scope,
		queryEmbedding,
		candidateK,
	)
	if err != nil {
		return nil, err
	}

	keywordHits, err := s.Store.KeywordSearchMemoryChunks(
		ctx,
		in.UserID,
		in.ConversationID,
		in.ProjectID,
		in.Scope,
		rewrittenQuery,
		candidateK,
	)
	if err != nil {
		return nil, err
	}

	merged := mergeAndDedupe(vectorHits, keywordHits)
	reranked := rerank(merged)
	filtered := applyPolicy(reranked, policy)

	log.Printf(
		"[Retrieval] conv=%s scope=%s query=%q topK=%d candidateK=%d vector_hits=%d keyword_hits=%d kept=%d min_final=%.2f",
		in.ConversationID,
		in.Scope,
		in.Query,
		policy.TopK,
		candidateK,
		len(vectorHits),
		len(keywordHits),
		len(filtered),
		policy.MinFinalScore,
	)

	return &RetrievedContext{
		Chunks: filtered,
	}, nil
}

func (s *Service) effectivePolicy(requestTopK int) Policy {
	p := s.Policy
	def := DefaultPolicy()

	if p.TopK <= 0 {
		p.TopK = def.TopK
	}
	if p.CandidateMultiplier <= 0 {
		p.CandidateMultiplier = def.CandidateMultiplier
	}
	if p.MinFinalScore <= 0 {
		p.MinFinalScore = def.MinFinalScore
	}
	if requestTopK > 0 {
		p.TopK = requestTopK
	}
	return p
}
