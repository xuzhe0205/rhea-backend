package retrieval

import "sort"

func rerank(chunks []RetrievedChunk) []RetrievedChunk {
	for i := range chunks {
		// v1 轻量加权
		// vector 为主，keyword 为辅，importance 先占很小权重
		chunks[i].FinalScore =
			0.70*chunks[i].VectorScore +
				0.25*chunks[i].KeywordScore +
				0.05*chunks[i].Chunk.ImportanceScore
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].FinalScore > chunks[j].FinalScore
	})

	return chunks
}
