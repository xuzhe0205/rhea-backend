package middleware

import (
	"log"
	"net/http"
)

// type MetricsMiddleware struct {
// 	Gemini *llm.GeminiProvider
// 	// 以后可以在这里加数据库连接，用来存 Token Usage
// }

func TokenUsageInterceptor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 请求前的逻辑 (例如日志记录)
		log.Printf("Hit TokenUsageInterceptor!")
		// 2. 调用下一个 Handler
		next.ServeHTTP(w, r)

		// 3. 请求后的逻辑 (例如在这里根据上下文记录统计)
		// 注意：如果你要在 Middleware 里拿 Gemini 做事（比如事后审计），
		// 你可以直接通过 m.Gemini 访问。
	})
}
