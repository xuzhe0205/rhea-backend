package retrieval

type Policy struct {
	TopK                int
	CandidateMultiplier int
	MinFinalScore       float64
	MinVectorScore      float64
	MinKeywordScore     float64
	RequireAnySignal    bool
}

func DefaultPolicy() Policy {
	return Policy{
		TopK:                8,
		CandidateMultiplier: 2,
		MinFinalScore:       0.35,
		MinVectorScore:      0.0,
		MinKeywordScore:     0.0,
		RequireAnySignal:    true,
	}
}
