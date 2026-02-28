package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

// CreateChain 辅助函数，把一堆中间件按顺序串起来
func CreateChain(middlewares ...Middleware) Middleware {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}
