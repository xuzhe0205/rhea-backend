package httpapi

import (
	"net/http"

	"rhea-backend/internal/auth"
	"rhea-backend/internal/storage"
)

type UploadHandler struct {
	R2 *storage.R2Client
}

// DeleteImage handles DELETE /v1/uploads/image?key=...
func (h *UploadHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "missing key")
		return
	}

	// Verify the key belongs to this user (keys are namespaced uploads/{userID}/...)
	expectedPrefix := "uploads/" + userID.String() + "/"
	if len(key) < len(expectedPrefix) || key[:len(expectedPrefix)] != expectedPrefix {
		respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := h.R2.DeleteImage(r.Context(), key); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete image")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UploadImage handles POST /v1/uploads/image
// Accepts multipart/form-data with a single "file" field.
// Returns { "key": "...", "url": "..." }
func (h *UploadHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Enforce total request size before parsing (20 MB + multipart overhead)
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxTotalSize+512*1024)

	if err := r.ParseMultipartForm(storage.MaxTotalSize); err != nil {
		respondError(w, http.StatusRequestEntityTooLarge, "request too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	if header.Size > storage.MaxImageSize {
		respondError(w, http.StatusRequestEntityTooLarge, "image exceeds 10 MB limit")
		return
	}

	contentType := header.Header.Get("Content-Type")

	key, err := h.R2.PutImage(r.Context(), userID.String(), header.Filename, contentType, file)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	url, err := h.R2.PresignGet(r.Context(), key)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate URL")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"key": key,
		"url": url,
	})
}
