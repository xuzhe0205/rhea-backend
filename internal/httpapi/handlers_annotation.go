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

func (h *AnnotationHandler) Annotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	var req model.Annotation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.UserID = uid

	newID, err := h.AnnSvc.AnnotateMessage(r.Context(), req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id": newID,
	})
}

func (h *AnnotationHandler) ListByMessage(w http.ResponseWriter, r *http.Request) {
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

func (h *AnnotationHandler) ListByConversation(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		respondError(w, http.StatusBadRequest, "missing conversation id")
		return
	}

	convID, err := uuid.Parse(parts[2])
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid conversation uuid")
		return
	}

	uid, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized: user not found in context")
		return
	}

	var messageIDs []uuid.UUID
	rawIDs := strings.TrimSpace(r.URL.Query().Get("message_ids"))
	if rawIDs != "" {
		for _, raw := range strings.Split(rawIDs, ",") {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid message_ids")
				return
			}
			messageIDs = append(messageIDs, id)
		}
	}

	list, err := h.AnnSvc.GetConversationAnnotations(r.Context(), convID, uid, messageIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *AnnotationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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