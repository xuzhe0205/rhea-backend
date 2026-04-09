package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"rhea-backend/internal/agent"
	"rhea-backend/internal/auth"
	"rhea-backend/internal/model"

	"github.com/google/uuid"
)

type ChatHandler struct {
	Agent *agent.Service
}

type chatRequest struct {
	ConversationID string   `json:"conversation_id"`
	Message        string   `json:"message"`
	ImageURLs      []string `json:"image_urls"`
}

type chatResponse struct {
	Reply          string `json:"reply"`
	ConversationID string `json:"conversation_id"` // 让前端能拿到新 ID
}

type patchFavoriteRequest struct {
	IsFavorite bool `json:"is_favorite"`
}

type patchFavoriteLabelRequest struct {
	FavoriteLabel string `json:"favorite_label"`
}

type PatchConversationPinRequest struct {
	IsPinned bool `json:"is_pinned"`
}

type conversationResponse struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	CumulativeTokens int    `json:"cumulative_tokens"`
	IsPinned         bool   `json:"is_pinned"`
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var req chatRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		// Case A: Payload too large
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Payload too large: limit is 1MB", http.StatusRequestEntityTooLarge)
			return
		}

		// Case B: Syntax error (e.g., missing comma, unclosed brace)
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			msg := fmt.Sprintf("Invalid JSON at byte offset %d", syntaxErr.Offset)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		// Case C: Type mismatch (e.g., sending a string where an int is expected)
		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &unmarshalErr) {
			msg := fmt.Sprintf("Field %q should be a %s", unmarshalErr.Field, unmarshalErr.Type)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		// Case D: Catch-all for other JSON/IO issues
		http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	reply, conversationID, err := h.Agent.Chat(r.Context(), req.ConversationID, req.Message)
	if err != nil {
		// 🚀 使用更稳健的错误识别
		if errors.Is(err, agent.ErrNoProvider) || strings.Contains(err.Error(), "no provider") {
			http.Error(w, err.Error(), http.StatusServiceUnavailable) // 503
			return
		}
		// 记录真实的错误到日志，方便调试，但返回 500 给前端
		log.Printf("[ChatHandler] Unexpected error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chatResponse{
		Reply:          reply,
		ConversationID: conversationID,
	})
}

// ListConversations 处理 GET /v1/conversations 请求
func (h *ChatHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	// 1. 验证方法（虽然路由层通常会限制，但 Handler 内部检查更稳妥）
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. 从 Context 提取 UserID (由 AuthMiddleware 注入)
	// 导入 "rhea-backend/internal/auth"
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	// 3. 调用 Service 层获取列表
	// 注意：确保你的 agent.Service 已经有了 ListUserConversations 方法
	convs, err := h.Agent.ListUserConversations(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to fetch conversations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. 返回结果
	w.Header().Set("Content-Type", "application/json")

	// 技巧：如果数据库没数据，convs 为 nil，JSON 序列化会变成 null。
	// 为了让前端 JavaScript 更好处理（直接 .length 或 .map），我们强制返回空数组 []。
	if convs == nil {
		convs = []*model.Conversation{}
	}

	_ = json.NewEncoder(w).Encode(convs)
}

// ListConversationMessages 处理 GET /v1/conversations/{id}/messages?limit=50&before_id=d12e534b...
func (h *ChatHandler) ListConversationMessages(w http.ResponseWriter, r *http.Request) {
	// 1. 验证方法
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. 提取 UserID
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	// 3. 提取路径参数 id
	// 注意：PathValue 是 Go 1.22+ 标准库 mux 的写法
	convIDStr := r.PathValue("id")
	if convIDStr == "" {
		http.Error(w, "missing conversation id", http.StatusBadRequest)
		return
	}

	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		http.Error(w, "invalid conversation id format", http.StatusBadRequest)
		return
	}

	// 4. 调用 Service 层获取消息列表
	// 遵循我们性能优先的决定，这里返回的是 []model.Message
	query := r.URL.Query()

	limitStr := query.Get("limit")
	limit := 50 // 默认值
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	beforeID := query.Get("before_id") // 如果没有传，这里就是 ""，DAO 已经能处理了
	msgs, err := h.Agent.ListConversationMessages(r.Context(), userID, convID, limit, beforeID)
	if err != nil {
		// 这里可以根据业务细化错误码，目前统一 500
		http.Error(w, "Failed to fetch messages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 5. 返回结果
	w.Header().Set("Content-Type", "application/json")

	// 同样确保返回 [] 而不是 null
	if msgs == nil {
		msgs = []model.Message{}
	}

	_ = json.NewEncoder(w).Encode(msgs)
}

// GetConversation 处理 GET /v1/conversations/{id}
func (h *ChatHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	convIDStr := r.PathValue("id")
	if convIDStr == "" {
		http.Error(w, "missing conversation id", http.StatusBadRequest)
		return
	}

	conv, err := h.Agent.GetConversation(r.Context(), convIDStr)
	if err != nil {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	if conv.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	resp := conversationResponse{
		ID:               conv.ID.String(),
		Title:            conv.Title,
		CumulativeTokens: conv.CumulativeTokens,
		IsPinned:         conv.IsPinned,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ChatHandler) PatchMessageFavorite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	messageIDStr := r.PathValue("id")
	if messageIDStr == "" {
		http.Error(w, "missing message id", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "invalid message id format", http.StatusBadRequest)
		return
	}

	var req patchFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			msg := fmt.Sprintf("Invalid JSON at byte offset %d", syntaxErr.Offset)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &unmarshalErr) {
			msg := fmt.Sprintf("Field %q should be a %s", unmarshalErr.Field, unmarshalErr.Type)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.Agent.SetMessageFavorite(r.Context(), userID, messageID, req.IsFavorite); err != nil {
		if strings.Contains(err.Error(), "access denied") {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update favorite: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message_id":  messageID,
		"is_favorite": req.IsFavorite,
		"updated":     true,
	})
}

func (h *ChatHandler) ListFavoriteMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()

	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	rows, err := h.Agent.ListFavoriteMessages(r.Context(), userID, limit, offset)
	if err != nil {
		http.Error(w, "Failed to fetch favorite messages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if rows == nil {
		rows = []model.FavoriteMessageRow{}
	}

	_ = json.NewEncoder(w).Encode(rows)
}

func (h *ChatHandler) ListMessagesForFavoriteJump(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	convIDStr := r.PathValue("id")
	if convIDStr == "" {
		http.Error(w, "missing conversation id", http.StatusBadRequest)
		return
	}

	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		http.Error(w, "invalid conversation id format", http.StatusBadRequest)
		return
	}

	messageIDStr := r.PathValue("messageId")
	if messageIDStr == "" {
		http.Error(w, "missing message id", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "invalid message id format", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()
	olderBuffer := 50
	if bufferStr := query.Get("older_buffer"); bufferStr != "" {
		if b, err := strconv.Atoi(bufferStr); err == nil && b >= 0 {
			olderBuffer = b
		}
	}

	msgs, err := h.Agent.ListMessagesForFavoriteJump(
		r.Context(),
		userID,
		convID,
		messageID,
		olderBuffer,
	)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch favorite jump messages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if msgs == nil {
		msgs = []model.Message{}
	}

	_ = json.NewEncoder(w).Encode(msgs)
}

func (h *ChatHandler) PatchMessageFavoriteLabel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	messageIDStr := r.PathValue("id")
	if messageIDStr == "" {
		http.Error(w, "missing message id", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "invalid message id format", http.StatusBadRequest)
		return
	}

	var req patchFavoriteLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	var labelPtr *string
	trimmed := strings.TrimSpace(req.FavoriteLabel)
	if trimmed != "" {
		labelPtr = &trimmed
	}

	if err := h.Agent.SetMessageFavoriteLabel(r.Context(), userID, messageID, labelPtr); err != nil {
		if strings.Contains(err.Error(), "access denied") {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update favorite label: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message_id":     messageID,
		"favorite_label": labelPtr,
		"updated":        true,
	})
}

func (h *ChatHandler) PatchConversationPin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	convIDStr := r.PathValue("id")
	if convIDStr == "" {
		http.Error(w, "missing conversation id", http.StatusBadRequest)
		return
	}

	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		http.Error(w, "invalid conversation id format", http.StatusBadRequest)
		return
	}

	var req PatchConversationPinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.Agent.SetConversationPinned(r.Context(), userID, convID, req.IsPinned); err != nil {
		if strings.Contains(err.Error(), "access denied") {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update conversation pin state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"conversation_id": convID,
		"is_pinned":       req.IsPinned,
		"updated":         true,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ChatHandler) ListPinnedConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: User not found in context", http.StatusUnauthorized)
		return
	}

	convs, err := h.Agent.ListPinnedConversations(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to list pinned conversations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if convs == nil {
		convs = []*model.Conversation{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(convs)
}
