package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

type TranscribeHandler struct{}

func (h *TranscribeHandler) Transcribe(w http.ResponseWriter, r *http.Request) {
	// 10 MB ceiling — ~1 hour of Opus audio is well under this
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "audio too large or malformed", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "missing audio field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read audio", http.StatusInternalServerError)
		return
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		http.Error(w, "transcription not configured", http.StatusInternalServerError)
		return
	}

	// Build multipart request to OpenAI Whisper
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", header.Filename)
	if err != nil {
		http.Error(w, "failed to build request", http.StatusInternalServerError)
		return
	}
	if _, err = fw.Write(audioData); err != nil {
		http.Error(w, "failed to build request", http.StatusInternalServerError)
		return
	}
	_ = mw.WriteField("model", "whisper-1")
	// If the caller specifies a language, forward it and add script-appropriate
	// prompts. Mandarin (zh) gets a Simplified-Chinese seed so Whisper outputs
	// Simplified characters instead of defaulting to Traditional.
	// Cantonese (yue) and others: auto-detect with no extra prompt.
	lang := r.URL.Query().Get("language")
	if lang != "" {
		_ = mw.WriteField("language", lang)
	}
	if lang == "zh" {
		_ = mw.WriteField("prompt", "以下是普通话的语音记录。")
	}
	mw.Close()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"https://api.openai.com/v1/audio/transcriptions", &buf)
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "transcription request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("whisper error: %s", string(body)), http.StatusBadGateway)
		return
	}

	var openAIResp struct {
		Text string `json:"text"`
	}
	if err = json.Unmarshal(body, &openAIResp); err != nil {
		http.Error(w, "failed to parse transcription response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"transcript": openAIResp.Text})
}
