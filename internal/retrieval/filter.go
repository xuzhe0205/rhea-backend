package retrieval

func applyPolicy(chunks []RetrievedChunk, policy Policy) []RetrievedChunk {
	if len(chunks) == 0 {
		return nil
	}

	out := make([]RetrievedChunk, 0, len(chunks))
	for _, ch := range chunks {
		if policy.RequireAnySignal && ch.VectorScore <= 0 && ch.KeywordScore <= 0 {
			continue
		}
		if policy.MinVectorScore > 0 && ch.VectorScore < policy.MinVectorScore {
			continue
		}
		if policy.MinKeywordScore > 0 && ch.KeywordScore < policy.MinKeywordScore {
			continue
		}
		if policy.MinFinalScore > 0 && ch.FinalScore < policy.MinFinalScore {
			continue
		}
		out = append(out, ch)
	}

	if policy.TopK > 0 && len(out) > policy.TopK {
		out = out[:policy.TopK]
	}

	return out
}
