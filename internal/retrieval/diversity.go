package retrieval

func diversify(chunks []RetrievedChunk, maxPerDocument int, maxAdjacentSpan int) []RetrievedChunk {
	if len(chunks) == 0 {
		return nil
	}
	if maxPerDocument <= 0 {
		maxPerDocument = 2
	}
	if maxAdjacentSpan <= 0 {
		maxAdjacentSpan = 1
	}

	out := make([]RetrievedChunk, 0, len(chunks))
	docCounts := make(map[string]int)
	lastChunkIndexByDoc := make(map[string][]int)

	for _, ch := range chunks {
		docID := ch.Chunk.DocumentID.String()

		if docCounts[docID] >= maxPerDocument {
			continue
		}

		skip := false
		for _, idx := range lastChunkIndexByDoc[docID] {
			diff := ch.Chunk.ChunkIndex - idx
			if diff < 0 {
				diff = -diff
			}
			if diff <= maxAdjacentSpan {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		out = append(out, ch)
		docCounts[docID]++
		lastChunkIndexByDoc[docID] = append(lastChunkIndexByDoc[docID], ch.Chunk.ChunkIndex)
	}

	return out
}
