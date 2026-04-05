package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"rhea-backend/internal/agent"
)

type ChatStreamHandler struct {
	Agent *agent.Service
	Limit int64 // max bytes for request body
}

func (h *ChatStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := h.Limit
	if limit <= 0 {
		limit = 1024 * 1024 // default 1MB
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	writeEvent := func(event, data string) error {
		_, err := fmt.Fprintf(w, "event: %s\n", event)
		if err != nil {
			return err
		}

		lines := strings.Split(data, "\n")
		for _, line := range lines {
			if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprint(w, "\n"); err != nil {
			return err
		}

		flusher.Flush()
		return nil
	}

	callbacks := agent.StreamCallbacks{
		OnDelta: func(delta string) error {
			return writeEvent("delta", delta)
		},
		OnMeta: func(payload map[string]any) error {
			b, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			return writeEvent("meta", string(b))
		},
		OnModel: func(model string) error {
			return writeEvent("model", model)
		},
	}

	_, err := h.Agent.ChatStream(r.Context(), req.ConversationID, req.Message, callbacks)
	if errors.Is(err, agent.ErrNoProvider) {
		_ = writeEvent("error", "no provider available")
		return
	}
	if err != nil {
		_ = writeEvent("error", err.Error())
		return
	}

	_ = writeEvent("done", "[DONE]")
}
