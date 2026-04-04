package retrieval

import (
	"rhea-backend/internal/store"
)

func mergeAndDedupe(
	vectorHits []store.MemoryChunkSearchResult,
	keywordHits []store.MemoryChunkSearchResult,
) []RetrievedChunk {
	byID := make(map[string]RetrievedChunk)

	for _, hit := range vectorHits {
		id := hit.Chunk.ID.String()
		byID[id] = RetrievedChunk{
			Chunk:        hit.Chunk,
			VectorScore:  hit.VectorScore,
			KeywordScore: hit.KeywordScore,
		}
	}

	for _, hit := range keywordHits {
		id := hit.Chunk.ID.String()

		if existing, ok := byID[id]; ok {
			if hit.VectorScore > existing.VectorScore {
				existing.VectorScore = hit.VectorScore
			}
			if hit.KeywordScore > existing.KeywordScore {
				existing.KeywordScore = hit.KeywordScore
			}
			byID[id] = existing
			continue
		}

		byID[id] = RetrievedChunk{
			Chunk:        hit.Chunk,
			VectorScore:  hit.VectorScore,
			KeywordScore: hit.KeywordScore,
		}
	}

	out := make([]RetrievedChunk, 0, len(byID))
	for _, ch := range byID {
		out = append(out, ch)
	}
	return out
}
