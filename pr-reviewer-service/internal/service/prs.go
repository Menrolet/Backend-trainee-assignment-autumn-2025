package service

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"time"

	"pr-reviewer-service/internal/model"
	"pr-reviewer-service/internal/repository"
)

type PRService struct {
	prs   *repository.PRsRepo
	users *repository.UsersRepo
	db    *sql.DB
}

func NewPRService(prs *repository.PRsRepo, users *repository.UsersRepo, db *sql.DB) *PRService {
	rand.Seed(time.Now().UnixNano())
	return &PRService{prs: prs, users: users, db: db}
}

// Create создает PR и назначает до двух активных ревьюверов из команды автора (без автора).
func (s *PRService) Create(ctx context.Context, id, name, authorID string) (model.PullRequest, error) {
	author, err := s.users.GetUser(ctx, authorID)
	if err != nil {
		return model.PullRequest{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
        SELECT user_id
        FROM users
        WHERE team_name=$1
          AND is_active=TRUE
          AND user_id <> $2
    `, author.TeamName, authorID)
	if err != nil {
		return model.PullRequest{}, err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return model.PullRequest{}, err
		}
		candidates = append(candidates, uid)
	}
	if err := rows.Err(); err != nil {
		return model.PullRequest{}, err
	}

	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	if len(candidates) > 2 {
		candidates = candidates[:2]
	}

	pr := model.PullRequest{
		ID:                id,
		Name:              name,
		AuthorID:          authorID,
		AssignedReviewers: candidates,
	}
	return s.prs.CreateWithReviewers(ctx, pr)
}

var (
	ErrPRMerged    = errors.New("pr merged")
	ErrNotAssigned = errors.New("reviewer not assigned")
	ErrNoCandidate = errors.New("no candidate")
)

func (s *PRService) Merge(ctx context.Context, prID string) (model.PullRequest, error) {
	return s.prs.Merge(ctx, prID)
}

func (s *PRService) Reassign(ctx context.Context, prID, oldUserID string) (model.PullRequest, string, error) {
	pr, err := s.prs.GetWithReviewers(ctx, prID)
	if err != nil {
		return model.PullRequest{}, "", err
	}
	if pr.Status == model.PRStatusMerged {
		return model.PullRequest{}, "", ErrPRMerged
	}

	assigned := false
	for _, r := range pr.AssignedReviewers {
		if r == oldUserID {
			assigned = true
			break
		}
	}
	if !assigned {
		return model.PullRequest{}, "", ErrNotAssigned
	}

	oldUser, err := s.users.GetUser(ctx, oldUserID)
	if err != nil {
		return model.PullRequest{}, "", err
	}

	// Ищем активного кандидата из команды старого ревьювера, исключая автора и уже назначенных.
	rows, err := s.db.QueryContext(ctx, `
        SELECT user_id
        FROM users
        WHERE team_name=$1
          AND is_active=TRUE
          AND user_id <> $2
          AND user_id <> $3
          AND user_id NOT IN (
              SELECT user_id FROM pull_request_reviewers WHERE pull_request_id=$4
          )
    `, oldUser.TeamName, oldUserID, pr.AuthorID, prID)
	if err != nil {
		return model.PullRequest{}, "", err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return model.PullRequest{}, "", err
		}
		candidates = append(candidates, uid)
	}
	if err := rows.Err(); err != nil {
		return model.PullRequest{}, "", err
	}
	if len(candidates) == 0 {
		return model.PullRequest{}, "", ErrNoCandidate
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	replacement := candidates[0]

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.PullRequest{}, "", err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
        DELETE FROM pull_request_reviewers
        WHERE pull_request_id=$1 AND user_id=$2
    `, prID, oldUserID); err != nil {
		return model.PullRequest{}, "", err
	}

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO pull_request_reviewers(pull_request_id, user_id)
        VALUES ($1,$2)
    `, prID, replacement); err != nil {
		return model.PullRequest{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return model.PullRequest{}, "", err
	}

	pr, err = s.prs.GetWithReviewers(ctx, prID)
	return pr, replacement, err
}

func (s *PRService) ListByReviewer(ctx context.Context, userID string) ([]model.PullRequest, error) {
	return s.prs.ListForReviewer(ctx, userID)
}
