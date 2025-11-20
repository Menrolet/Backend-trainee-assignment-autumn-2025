package http

import (
	"encoding/json"
	"net/http"

	"pr-reviewer-service/internal/model"
	"pr-reviewer-service/internal/repository"
	"pr-reviewer-service/internal/service"
)

type Handler struct {
	teams *service.TeamsService
	users *service.UsersService
	prs   *service.PRService
}

func NewHandler(teams *service.TeamsService, users *service.UsersService, prs *service.PRService) *Handler {
	return &Handler{teams: teams, users: users, prs: prs}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) AddTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	var t model.Team
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, CodeNotFound, "bad json")
		return
	}
	team, err := h.teams.Create(r.Context(), t)
	if err != nil {
		switch err {
		case repository.ErrTeamExists:
			writeError(w, http.StatusBadRequest, CodeTeamExists, "team_name already exists")
		default:
			writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"team": team})
}

func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	name := r.URL.Query().Get("team_name")
	if name == "" {
		writeError(w, http.StatusBadRequest, CodeNotFound, "team_name is required")
		return
	}
	team, err := h.teams.Get(r.Context(), name)
	if err != nil {
		if err == repository.ErrNotFound {
			writeError(w, http.StatusNotFound, CodeNotFound, "resource not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (h *Handler) SetUserActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	var req struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeNotFound, "bad json")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, CodeNotFound, "user_id is required")
		return
	}
	user, err := h.users.SetIsActive(r.Context(), req.UserID, req.IsActive)
	if err != nil {
		if err == repository.ErrNotFound {
			writeError(w, http.StatusNotFound, CodeNotFound, "resource not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) CreatePR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	var req struct {
		ID     string `json:"pull_request_id"`
		Name   string `json:"pull_request_name"`
		Author string `json:"author_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeNotFound, "bad json")
		return
	}
	if req.ID == "" || req.Name == "" || req.Author == "" {
		writeError(w, http.StatusBadRequest, CodeNotFound, "missing required fields")
		return
	}
	pr, err := h.prs.Create(r.Context(), req.ID, req.Name, req.Author)
	if err != nil {
		switch err {
		case repository.ErrNotFound:
			writeError(w, http.StatusNotFound, CodeNotFound, "resource not found")
		case repository.ErrPRExists:
			writeError(w, http.StatusConflict, CodePRExists, "PR id already exists")
		default:
			writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pr": pr})
}

func (h *Handler) MergePR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"pull_request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeNotFound, "bad json")
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, CodeNotFound, "pull_request_id is required")
		return
	}
	pr, err := h.prs.Merge(r.Context(), req.ID)
	if err != nil {
		if err == repository.ErrNotFound {
			writeError(w, http.StatusNotFound, CodeNotFound, "resource not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pr": pr})
}

func (h *Handler) Reassign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	var req struct {
		PRID    string `json:"pull_request_id"`
		OldUser string `json:"old_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeNotFound, "bad json")
		return
	}
	if req.PRID == "" || req.OldUser == "" {
		writeError(w, http.StatusBadRequest, CodeNotFound, "missing required fields")
		return
	}
	pr, replacement, err := h.prs.Reassign(r.Context(), req.PRID, req.OldUser)
	if err != nil {
		switch err {
		case repository.ErrNotFound:
			writeError(w, http.StatusNotFound, CodeNotFound, "resource not found")
		case service.ErrPRMerged:
			writeError(w, http.StatusConflict, CodePRMerged, "cannot reassign on merged PR")
		case service.ErrNotAssigned:
			writeError(w, http.StatusConflict, CodeNotAssigned, "reviewer is not assigned to this PR")
		case service.ErrNoCandidate:
			writeError(w, http.StatusConflict, CodeNoCandidate, "no active replacement candidate in team")
		default:
			writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pr":          pr,
		"replaced_by": replacement,
	})
}

func (h *Handler) GetReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, CodeNotFound, "user_id is required")
		return
	}
	// ensure user exists
	if _, err := h.users.Get(r.Context(), userID); err != nil {
		if err == repository.ErrNotFound {
			writeError(w, http.StatusNotFound, CodeNotFound, "resource not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		return
	}
	prs, err := h.prs.ListByReviewer(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		return
	}
	// build short form
	type prShort struct {
		ID     string         `json:"pull_request_id"`
		Name   string         `json:"pull_request_name"`
		Author string         `json:"author_id"`
		Status model.PRStatus `json:"status"`
	}
	var resp []prShort
	for _, pr := range prs {
		resp = append(resp, prShort{
			ID:     pr.ID,
			Name:   pr.Name,
			Author: pr.AuthorID,
			Status: pr.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":       userID,
		"pull_requests": resp,
	})
}

func (h *Handler) ReviewerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, CodeNotFound, "method not allowed")
		return
	}
	stats, err := h.prs.ReviewerStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reviewer_assignments": stats,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
