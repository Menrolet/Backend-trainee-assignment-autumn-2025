package http

import (
	"net/http"
)

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/team/add", h.AddTeam)
	mux.HandleFunc("/team/get", h.GetTeam)
	mux.HandleFunc("/team/deactivate", h.DeactivateTeam)
	mux.HandleFunc("/users/setIsActive", h.SetUserActive)
	mux.HandleFunc("/pullRequest/create", h.CreatePR)
	mux.HandleFunc("/pullRequest/merge", h.MergePR)
	mux.HandleFunc("/pullRequest/reassign", h.Reassign)
	mux.HandleFunc("/users/getReview", h.GetReviews)
	mux.HandleFunc("/stats/reviewerAssignments", h.ReviewerStats)

	return mux
}
