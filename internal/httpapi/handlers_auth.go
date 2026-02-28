package httpapi

import (
	"encoding/json"
	"net/http"
	"rhea-backend/internal/auth"
)

type AuthHandler struct {
	AuthSvc *auth.Service
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.AuthSvc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 返回成功，但不包含 PasswordHash
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 同样的解析逻辑...
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, err := h.AuthSvc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	// 1. 从 Context 中提取身份 (由 AuthMiddleware 注入)
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: No user ID in context", http.StatusUnauthorized)
		return
	}

	// 2. 调用 Service 获取完整用户实体
	user, err := h.AuthSvc.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// 3. 返回 JSON (过滤掉敏感字段如 PasswordHash)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         user.ID,
		"email":      user.Email,
		"user_name":  user.UserName,
		"created_at": user.CreatedAt,
		// 预留字段，方便以后扩展 profile
		"metadata": user.Metadata,
	})
}
