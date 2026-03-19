package httpapi

import (
	"encoding/json"
	"net/http"
	
	"rhea-backend/internal/auth"
	"rhea-backend/internal/model"
	"rhea-backend/internal/service"
	"strings"

	"github.com/google/uuid"
)

type AnnotationHandler struct {
	AnnSvc *service.AnnotationService
}

// Annotate 处理 POST /v1/annotations (创建或更新)
func (h *AnnotationHandler) Annotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. 获取 UserID (从 Middleware 注入的 Context 中拿)
	uid, ok := auth.GetUserID(r.Context()) 
	if !ok {
		// 这里的 respondError 就会被跳过，如果 ok 为 true 的话
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	// 2. 解析请求体
	var req model.Annotation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.UserID = uid // 强制锁死为当前登录用户

	// 3. 调用 Service
	newID, err := h.AnnSvc.AnnotateMessage(r.Context(), req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id": newID,
	})
}

// ListByMessage 处理 GET /v1/messages/{id}/annotations
func (h *AnnotationHandler) ListByMessage(w http.ResponseWriter, r *http.Request) {
	// 简单的路由参数提取 (假设格式为 /v1/messages/UUID/annotations)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		respondError(w, http.StatusBadRequest, "missing message id")
		return
	}
	msgID, err := uuid.Parse(parts[2])
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid message uuid")
		return
	}

	uid, ok := auth.GetUserID(r.Context()) 
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	list, err := h.AnnSvc.GetMessageAnnotations(r.Context(), msgID, uid)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, list)
}

// Delete 处理 DELETE /v1/annotations/{id}
func (h *AnnotationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 提取 ID
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	annID, err := uuid.Parse(parts[len(parts)-1])
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid annotation uuid")
		return
	}

	uid, ok := auth.GetUserID(r.Context()) 
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	if err := h.AnnSvc.RemoveAnnotation(r.Context(), annID, uid); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
