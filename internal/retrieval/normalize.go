package retrieval

import (
	"regexp"
	"strings"
)

var punctuationRE = regexp.MustCompile(`[^\pL\pN\s]+`)

var stopPhrases = []string{
	"can you",
	"could you",
	"would you",
	"please",
	"briefly",
	"again",
	"what exactly is",
	"what is",
	"explain",
	"tell me",
}

func normalizeVectorQuery(q string) string {
	q = strings.TrimSpace(strings.ToLower(q))
	q = punctuationRE.ReplaceAllString(q, " ")
	q = strings.Join(strings.Fields(q), " ")
	return q
}

func normalizeKeywordQuery(q string) string {
	q = normalizeVectorQuery(q)

	for _, phrase := range stopPhrases {
		q = strings.ReplaceAll(q, phrase, " ")
	}

	q = strings.Join(strings.Fields(q), " ")
	return q
}

func NormalizeFTSConfig(cfg string) string {
	switch cfg {
	case "english", "simple":
		return cfg
	default:
		return "english"
	}
}
