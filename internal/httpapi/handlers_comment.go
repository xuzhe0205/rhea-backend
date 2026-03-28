package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"rhea-backend/internal/auth"
	"rhea-backend/internal/model"
	"rhea-backend/internal/service"

	"github.com/google/uuid"
)

type CommentHandler struct {
	CommentSvc *service.CommentService
}

func NewCommentHandler(commentSvc *service.CommentService) *CommentHandler {
	return &CommentHandler{
		CommentSvc: commentSvc,
	}
}

func (h *CommentHandler) ListByMessageIDs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	rawIDs := strings.TrimSpace(r.URL.Query().Get("message_ids"))
	if rawIDs == "" {
		respondJSON(w, http.StatusOK, []*model.CommentThread{})
		return
	}

	var messageIDs []uuid.UUID
	for _, raw := range strings.Split(rawIDs, ",") {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid message_ids")
			return
		}
		messageIDs = append(messageIDs, id)
	}

	list, err := h.CommentSvc.GetCommentThreadsByMessageIDs(r.Context(), uid, messageIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *CommentHandler) GetCommentThread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	msgIDStr := strings.TrimSpace(r.URL.Query().Get("message_id"))
	startStr := strings.TrimSpace(r.URL.Query().Get("range_start"))
	endStr := strings.TrimSpace(r.URL.Query().Get("range_end"))

	msgID, err := uuid.Parse(msgIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid message_id")
		return
	}

	start, err := strconv.Atoi(startStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid range_start")
		return
	}

	end, err := strconv.Atoi(endStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid range_end")
		return
	}

	thread, err := h.CommentSvc.GetCommentThread(r.Context(), msgID, uid, start, end)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if thread == nil {
		respondError(w, http.StatusNotFound, "comment thread not found")
		return
	}

	respondJSON(w, http.StatusOK, thread)
}

func (h *CommentHandler) GetComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	commentID, err := uuid.Parse(parts[len(parts)-1])
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid comment uuid")
		return
	}

	comment, err := h.CommentSvc.GetComment(r.Context(), commentID, uid)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if comment == nil {
		respondError(w, http.StatusNotFound, "comment not found")
		return
	}

	respondJSON(w, http.StatusOK, comment)
}

func (h *CommentHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	var req model.AddCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}

	thread, comment, err := h.CommentSvc.AddComment(r.Context(), uid, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"thread":  thread,
		"comment": comment,
	})
}

func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	commentID, err := uuid.Parse(parts[len(parts)-1])
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid comment uuid")
		return
	}

	if err := h.CommentSvc.DeleteComment(r.Context(), commentID, uid); err != nil {
		if err.Error() == "comment not found" {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
