package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"rhea-backend/internal/auth"
	"rhea-backend/internal/model"
	"rhea-backend/internal/service"

	"github.com/google/uuid"
)

// ProjectHandler handles all /v1/projects routes.
type ProjectHandler struct {
	ProjectSvc *service.ProjectService
}

// ─── Request / Response types ────────────────────────────────────────────────

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createProjectConversationRequest struct {
	// Message is the user's first message; used to derive the conversation title.
	Message string `json:"message"`
	Title   string `json:"title"`
}

type projectResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Summary     string `json:"summary"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type projectConversationResponse struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	CumulativeTokens int    `json:"cumulative_tokens"`
	IsPinned         bool   `json:"is_pinned"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func projectToResponse(p *model.Project) projectResponse {
	return projectResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		Summary:     p.Summary,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func convToProjectResponse(c *model.Conversation) projectConversationResponse {
	return projectConversationResponse{
		ID:               c.ID.String(),
		Title:            c.Title,
		CumulativeTokens: c.CumulativeTokens,
		IsPinned:         c.IsPinned,
		CreatedAt:        c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:        c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func getUserIDOrReject(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	return userID, ok
}

func handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrForbidden) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if errors.Is(err, service.ErrProjectNotEmpty) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// ListProjects handles GET /v1/projects
func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	projects, err := h.ProjectSvc.ListProjects(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := make([]projectResponse, len(projects))
	for i, p := range projects {
		resp[i] = projectToResponse(p)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateProject handles POST /v1/projects
func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	p, err := h.ProjectSvc.CreateProject(r.Context(), userID, req.Name, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(projectToResponse(p))
}

// GetProject handles GET /v1/projects/{id}
func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	projectID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	p, err := h.ProjectSvc.GetProject(r.Context(), userID, projectID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(projectToResponse(p))
}

// UpdateProject handles PATCH /v1/projects/{id}
func (h *ProjectHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	projectID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	p, err := h.ProjectSvc.UpdateProject(r.Context(), userID, projectID, req.Name, req.Description)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(projectToResponse(p))
}

// DeleteProject handles DELETE /v1/projects/{id}
func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	projectID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	if err := h.ProjectSvc.DeleteProject(r.Context(), userID, projectID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListProjectConversations handles GET /v1/projects/{id}/conversations
func (h *ProjectHandler) ListProjectConversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	projectID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	convs, err := h.ProjectSvc.ListProjectConversations(r.Context(), userID, projectID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if convs == nil {
		convs = []*model.Conversation{}
	}

	resp := make([]projectConversationResponse, len(convs))
	for i, c := range convs {
		resp[i] = convToProjectResponse(c)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateProjectConversation handles POST /v1/projects/{id}/conversations
func (h *ProjectHandler) CreateProjectConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserIDOrReject(w, r)
	if !ok {
		return
	}

	projectID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	var req createProjectConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	// Derive title from explicit title field or fall back to the first message text.
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSpace(req.Message)
	}
	if title == "" {
		title = "New Thread"
	}
	// Truncate long titles derived from message content.
	if runes := []rune(title); len(runes) > 60 {
		title = string(runes[:60]) + "…"
	}

	conv, err := h.ProjectSvc.CreateProjectConversation(r.Context(), userID, projectID, title)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(convToProjectResponse(conv))
}
