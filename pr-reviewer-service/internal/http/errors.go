package http

import (
	"encoding/json"
	"net/http"
)

type errorCode string

const (
	CodeTeamExists  errorCode = "TEAM_EXISTS"
	CodePRExists    errorCode = "PR_EXISTS"
	CodePRMerged    errorCode = "PR_MERGED"
	CodeNotAssigned errorCode = "NOT_ASSIGNED"
	CodeNoCandidate errorCode = "NO_CANDIDATE"
	CodeNotFound    errorCode = "NOT_FOUND"
)

type errorResponse struct {
	Error struct {
		Code    errorCode `json:"code"`
		Message string    `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code errorCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	var resp errorResponse
	resp.Error.Code = code
	resp.Error.Message = msg
	_ = json.NewEncoder(w).Encode(resp)
}
