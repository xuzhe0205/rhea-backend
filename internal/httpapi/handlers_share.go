package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"rhea-backend/internal/auth"
	"rhea-backend/internal/model"
	"rhea-backend/internal/store"
)

const (
	shareTokenChars  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	shareTokenLength = 12
	maxShareMessages = 20
)

type ShareHandler struct {
	Store store.Store
	R2    interface {
		PresignGet(ctx context.Context, key string) (string, error)
	}
}

// POST /v1/share
type createShareRequest struct {
	MessageIDs []string `json:"message_ids"`
}

type createShareResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

func (h *ShareHandler) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var req createShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if len(req.MessageIDs) == 0 {
		http.Error(w, "message_ids is required", http.StatusBadRequest)
		return
	}
	if len(req.MessageIDs) > maxShareMessages {
		http.Error(w, fmt.Sprintf("max %d messages per share link", maxShareMessages), http.StatusBadRequest)
		return
	}

	// Parse and deduplicate UUIDs, preserving order
	seen := make(map[uuid.UUID]bool, len(req.MessageIDs))
	msgIDs := make([]uuid.UUID, 0, len(req.MessageIDs))
	for _, idStr := range req.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "invalid message id: "+idStr, http.StatusBadRequest)
			return
		}
		if !seen[id] {
			seen[id] = true
			msgIDs = append(msgIDs, id)
		}
	}

	// Fetch messages to verify they exist
	msgs, err := h.Store.GetMessagesByIDs(r.Context(), msgIDs)
	if err != nil || len(msgs) != len(msgIDs) {
		http.Error(w, "one or more messages not found", http.StatusNotFound)
		return
	}

	// Verify ownership: all messages must belong to conversations owned by this user
	convIDs := make(map[uuid.UUID]bool)
	for _, m := range msgs {
		convIDs[m.ConvID] = true
	}
	for convID := range convIDs {
		conv, err := h.Store.GetConversation(r.Context(), convID.String())
		if err != nil || conv.UserID != userID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	token, err := generateShareToken()
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	link := &model.ShareLink{
		ID:            uuid.New(),
		Token:         token,
		CreatorUserID: userID,
		MessageIDs:    msgIDs,
		CreatedAt:     time.Now().UTC(),
	}
	if err := h.Store.CreateShareLink(r.Context(), link); err != nil {
		http.Error(w, "failed to create share link", http.StatusInternalServerError)
		return
	}

	shareURL := buildShareURL(r, token)
	respondJSON(w, http.StatusCreated, createShareResponse{Token: token, URL: shareURL})
}

// GET /v1/share/:token  — public, no auth
type sharedMessageDTO struct {
	ID        string                 `json:"id"`
	Role      model.Role             `json:"role"`
	Content   string                 `json:"content"`
	CreatedAt time.Time              `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type getShareResponse struct {
	Messages []sharedMessageDTO `json:"messages"`
	SharedAt time.Time          `json:"shared_at"`
}

func (h *ShareHandler) GetSharedContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.PathValue("token")
	// Validate token format before hitting the DB
	if token == "" || len(token) > 32 || !isValidShareToken(token) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	link, err := h.Store.GetShareLinkByToken(r.Context(), token)
	if err != nil {
		// Always 404 — never distinguish "revoked" from "not found"
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	msgs, err := h.Store.GetMessagesByIDs(r.Context(), link.MessageIDs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Re-presign image keys so shared pages always show fresh image URLs
	if h.R2 != nil {
		for i := range msgs {
			if msgs[i].Metadata == nil {
				continue
			}
			raw, ok := msgs[i].Metadata["image_keys"]
			if !ok {
				continue
			}
			keys, ok := extractStringSlice(raw)
			if !ok || len(keys) == 0 {
				continue
			}
			urls := make([]string, 0, len(keys))
			for _, k := range keys {
				u, err := h.R2.PresignGet(r.Context(), k)
				if err == nil {
					urls = append(urls, u)
				}
			}
			msgs[i].Metadata["image_urls"] = urls
		}
	}

	dtos := make([]sharedMessageDTO, 0, len(msgs))
	for _, m := range msgs {
		dtos = append(dtos, sharedMessageDTO{
			ID:        m.ID.String(),
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
			Metadata:  sanitizeMetadataForShare(m.Metadata),
		})
	}

	w.Header().Set("Cache-Control", "public, max-age=300") // 5-minute CDN cache
	respondJSON(w, http.StatusOK, getShareResponse{
		Messages: dtos,
		SharedAt: link.CreatedAt,
	})
}

// sanitizeMetadataForShare strips internal fields (image_keys) and only exposes
// what viewers need (image_urls for display).
func sanitizeMetadataForShare(meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		return nil
	}
	out := make(map[string]interface{})
	if urls, ok := meta["image_urls"]; ok {
		out["image_urls"] = urls
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func generateShareToken() (string, error) {
	b := make([]byte, shareTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = shareTokenChars[int(b[i])%len(shareTokenChars)]
	}
	return string(b), nil
}

func isValidShareToken(token string) bool {
	for _, c := range token {
		if !strings.ContainsRune(shareTokenChars, c) {
			return false
		}
	}
	return true
}

// buildShareURL constructs the frontend share URL from the request host.
// In production this will be rheaindex.com; in dev, localhost:3000.
func buildShareURL(r *http.Request, token string) string {
	scheme := "https"
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	// Dev: backend is on :8080, frontend on :3000
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.") {
		host = strings.Replace(host, "8080", "3000", 1)
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/s/%s", scheme, host, token)
}

