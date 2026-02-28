package middleware

import (
	"net/http"
	"strings"

	"rhea-backend/internal/auth"
)

// AuthMiddleware 是我们的保安函数
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 从 Header 拿到 Authorization: Bearer <token>
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		// 2. 截取掉 "Bearer " 前缀
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		// 3. 解析并验证 Token (调用你之前写好的 ParseToken)
		userID, err := auth.ParseToken(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// 4. 【关键】将 UserID 塞进请求的 Context 中，并传给下一个 Handler
		ctx := auth.SetUserID(r.Context(), userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
