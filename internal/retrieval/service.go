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

	vectorQuery := normalizeVectorQuery(rewrittenQuery)
	keywordQuery := normalizeKeywordQuery(rewrittenQuery)

	candidateK := policy.TopK * policy.CandidateMultiplier
	if candidateK < policy.TopK {
		candidateK = policy.TopK
	}

	threshold := policy.CJKThreshold
	if threshold <= 0 {
		threshold = DefaultPolicy().CJKThreshold
	}
	ratio := cjkRatio(in.Query)
	skipFTS := ratio >= threshold

	queryEmbedding, err := s.Embeddings.EmbedText(ctx, vectorQuery)
	if err != nil {
		return nil, err
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

	var keywordHits []store.MemoryChunkSearchResult
	if !skipFTS {
		keywordHits, err = s.Store.KeywordSearchMemoryChunks(
			ctx,
			in.UserID,
			in.ConversationID,
			in.ProjectID,
			in.Scope,
			keywordQuery,
			policy.FTSConfig,
			candidateK,
		)
		if err != nil {
			return nil, err
		}
	}

	merged := mergeAndDedupe(vectorHits, keywordHits)
	reranked := rerank(merged, skipFTS)
	diverse := diversify(reranked, 2, 1)
	filtered := applyPolicy(diverse, policy)

	log.Printf(
		"[Retrieval] conv=%s scope=%s query=%q vector_query=%q keyword_query=%q fts=%s skip_fts=%v cjk_ratio=%.2f topK=%d candidateK=%d vector_hits=%d keyword_hits=%d kept=%d min_final=%.2f",
		in.ConversationID,
		in.Scope,
		in.Query,
		vectorQuery,
		keywordQuery,
		policy.FTSConfig,
		skipFTS,
		ratio,
		policy.TopK,
		candidateK,
		len(vectorHits),
		len(keywordHits),
		len(filtered),
		policy.MinFinalScore,
	)

	for i, ch := range filtered {
		if i >= 3 {
			break
		}
		log.Printf(
			"[Retrieval] kept[%d] score=%.4f vector=%.4f keyword=%.4f doc=%s chunk=%d source=%s",
			i,
			ch.FinalScore,
			ch.VectorScore,
			ch.KeywordScore,
			ch.Chunk.DocumentID,
			ch.Chunk.ChunkIndex,
			ch.Chunk.SourceType,
		)
	}

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
	if p.MinVectorScore <= 0 {
		p.MinVectorScore = def.MinVectorScore
	}
	if p.MinKeywordScore <= 0 {
		p.MinKeywordScore = def.MinKeywordScore
	}
	if p.FTSConfig == "" {
		p.FTSConfig = def.FTSConfig
	}
	// RequireAnySignal 默认值是 true，不能靠 <=0 判断
	if !p.RequireAnySignal {
		p.RequireAnySignal = def.RequireAnySignal
	}

	if requestTopK > 0 {
		p.TopK = requestTopK
	}
	return p
}
