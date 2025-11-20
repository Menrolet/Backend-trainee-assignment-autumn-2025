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

func (s *PRService) ReviewerStats(ctx context.Context) ([]model.ReviewerStat, error) {
	return s.prs.CountAssignmentsByReviewer(ctx)
}

type DeactivateResult struct {
	Team           string   `json:"team_name"`
	Deactivated    []string `json:"deactivated_user_ids"`
	Reassigned     int      `json:"reassigned"`
	UnassignedLeft int      `json:"unassigned_left"`
}

// DeactivateTeam массово деактивирует пользователей команды и старается заменить их в открытых PR.
// Если кандидатов нет, ревьювер просто снимается.
func (s *PRService) DeactivateTeam(ctx context.Context, team string) (DeactivateResult, error) {
	users, err := s.users.ListByTeam(ctx, team)
	if err != nil {
		return DeactivateResult{}, err
	}
	if len(users) == 0 {
		return DeactivateResult{}, repository.ErrNotFound
	}
	deactivated := make(map[string]bool)
	for _, u := range users {
		deactivated[u.UserID] = true
	}

	// Найдём открытые PR, где есть ревьюверы из этой команды.
	rows, err := s.db.QueryContext(ctx, `
        SELECT DISTINCT pr.pull_request_id
        FROM pull_requests pr
        JOIN pull_request_reviewers r ON pr.pull_request_id = r.pull_request_id
        JOIN users u ON u.user_id = r.user_id
        WHERE pr.status = 'OPEN' AND u.team_name = $1
    `, team)
	if err != nil {
		return DeactivateResult{}, err
	}
	var prIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return DeactivateResult{}, err
		}
		prIDs = append(prIDs, id)
	}
	if err := rows.Err(); err != nil {
		return DeactivateResult{}, err
	}
	rows.Close()

	result := DeactivateResult{Team: team}

	// Обрабатываем PR до смены статуса is_active, чтобы ещё можно было выбрать кандидатов из других команд.
	for _, prID := range prIDs {
		pr, err := s.prs.GetWithReviewers(ctx, prID)
		if err != nil {
			return DeactivateResult{}, err
		}
		assignedSet := make(map[string]bool)
		for _, rid := range pr.AssignedReviewers {
			assignedSet[rid] = true
		}
		for _, rid := range pr.AssignedReviewers {
			if !deactivated[rid] {
				continue
			}
			// снять старого
			if err := s.prs.RemoveReviewer(ctx, prID, rid); err != nil {
				return DeactivateResult{}, err
			}
			result.Deactivated = append(result.Deactivated, rid)
			delete(assignedSet, rid)

			// подобрать замену среди активных пользователей той же команды, не автора и не уже назначенных/деактивируемых
			candidate, err := s.findReplacement(ctx, team, pr.AuthorID, assignedSet, deactivated)
			if err != nil {
				return DeactivateResult{}, err
			}
			if candidate != "" {
				if err := s.prs.AddReviewer(ctx, prID, candidate); err != nil {
					return DeactivateResult{}, err
				}
				assignedSet[candidate] = true
				result.Reassigned++
			} else {
				result.UnassignedLeft++
			}
		}
	}

	// Теперь деактивируем всех пользователей команды.
	_, err = s.db.ExecContext(ctx, `UPDATE users SET is_active=FALSE WHERE team_name=$1`, team)
	if err != nil {
		return DeactivateResult{}, err
	}
	return result, nil
}

func (s *PRService) findReplacement(ctx context.Context, team, author string, assigned map[string]bool, deactivated map[string]bool) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT user_id
        FROM users
        WHERE team_name=$1 AND is_active=TRUE AND user_id <> $2
    `, team, author)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var pool []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return "", err
		}
		if assigned[uid] || deactivated[uid] {
			continue
		}
		pool = append(pool, uid)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pool) == 0 {
		return "", nil
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return pool[0], nil
}
