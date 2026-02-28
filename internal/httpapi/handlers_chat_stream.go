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

		// Split on \n and write one data: line per line.
		lines := strings.Split(data, "\n")
		for _, line := range lines {
			if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
				return err
			}
		}

		// End of event
		if _, err := fmt.Fprint(w, "\n"); err != nil {
			return err
		}

		flusher.Flush()
		return nil
	}

	convID, err := h.Agent.ChatStream(r.Context(), req.ConversationID, req.Message, func(delta string) error {
		return writeEvent("delta", delta)
	})

	if errors.Is(err, agent.ErrNoProvider) {
		_ = writeEvent("error", "no provider available")
		return
	}
	if err != nil {
		_ = writeEvent("error", err.Error())
		return
	}

	if convID != "" {
		// 这里的逻辑是：既然 AI 已经说完了，标题生成大概率也跑完了（标题生成通常比长回复快）
		// 我们从数据库拉取最新的对话元数据
		// 建议在 Service 里封装一个 GetConversation 方法
		conv, err := h.Agent.GetConversation(r.Context(), convID)
		if err == nil && conv.Title != "" {
			// 发送一个新的事件类型：title
			_ = writeEvent("title", conv.Title)
		}
		_ = writeEvent("conv_id", convID)
	}

	_ = writeEvent("done", "[DONE]")
}
