package retrieval

import "strings"

func rewriteQuery(q string) string {
	q = strings.TrimSpace(q)
	return q
}
