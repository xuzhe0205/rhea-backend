package retrieval

import (
	"regexp"
	"strings"
)

var punctuationRE = regexp.MustCompile(`[^\pL\pN\s]+`)

// cjkRatio returns the fraction of runes in s that are CJK, Japanese kana, or Korean Hangul.
func cjkRatio(s string) float64 {
	total, cjk := 0, 0
	for _, r := range s {
		total++
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
			(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
			(r >= 0x3040 && r <= 0x30FF) || // Hiragana + Katakana
			(r >= 0xAC00 && r <= 0xD7A3) { // Hangul syllables
			cjk++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(cjk) / float64(total)
}

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
