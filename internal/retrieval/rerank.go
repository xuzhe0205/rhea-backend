package retrieval

import "sort"

func rerank(chunks []RetrievedChunk, skipFTS bool) []RetrievedChunk {
	for i := range chunks {
		if skipFTS {
			// FTS skipped (CJK/non-Latin query): redistribute keyword weight to vector
			chunks[i].FinalScore =
				0.95*chunks[i].VectorScore +
					0.05*chunks[i].Chunk.ImportanceScore
		} else {
			chunks[i].FinalScore =
				0.70*chunks[i].VectorScore +
					0.25*chunks[i].KeywordScore +
					0.05*chunks[i].Chunk.ImportanceScore
		}
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].FinalScore > chunks[j].FinalScore
	})

	return chunks
}
