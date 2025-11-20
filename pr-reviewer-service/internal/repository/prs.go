package repository

import (
	"context"
	"database/sql"
	"errors"

	"pr-reviewer-service/internal/model"
)

type PRsRepo struct{ db *sql.DB }

func NewPRsRepo(db *sql.DB) *PRsRepo { return &PRsRepo{db: db} }

var ErrPRExists = errors.New("pr exists")

func (r *PRsRepo) CreateWithReviewers(ctx context.Context, pr model.PullRequest) (model.PullRequest, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.PullRequest{}, err
	}
	defer tx.Rollback()

	var tmp string
	err = tx.QueryRowContext(ctx, `SELECT pull_request_id FROM pull_requests WHERE pull_request_id=$1`, pr.ID).
		Scan(&tmp)
	if err == nil {
		return model.PullRequest{}, ErrPRExists
	}
	if err != nil && err != sql.ErrNoRows {
		return model.PullRequest{}, err
	}

	err = tx.QueryRowContext(ctx, `
        INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status)
        VALUES ($1,$2,$3,'OPEN')
        RETURNING created_at
    `, pr.ID, pr.Name, pr.AuthorID).Scan(&pr.CreatedAt)
	if err != nil {
		return model.PullRequest{}, err
	}
	pr.Status = model.PRStatusOpen

	for _, rid := range pr.AssignedReviewers {
		if _, err := tx.ExecContext(ctx, `
           INSERT INTO pull_request_reviewers (pull_request_id, user_id)
           VALUES ($1,$2)
        `, pr.ID, rid); err != nil {
			return model.PullRequest{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return model.PullRequest{}, err
	}
	return pr, nil
}

func (r *PRsRepo) GetWithReviewers(ctx context.Context, id string) (model.PullRequest, error) {
	var pr model.PullRequest
	err := r.db.QueryRowContext(ctx, `
        SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
        FROM pull_requests WHERE pull_request_id=$1`, id).
		Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &pr.MergedAt)
	if err == sql.ErrNoRows {
		return model.PullRequest{}, ErrNotFound
	}
	if err != nil {
		return model.PullRequest{}, err
	}

	rows, err := r.db.QueryContext(ctx, `
        SELECT user_id FROM pull_request_reviewers WHERE pull_request_id=$1`, id)
	if err != nil {
		return model.PullRequest{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return model.PullRequest{}, err
		}
		pr.AssignedReviewers = append(pr.AssignedReviewers, uid)
	}
	if err := rows.Err(); err != nil {
		return model.PullRequest{}, err
	}
	return pr, nil
}

func (r *PRsRepo) Merge(ctx context.Context, id string) (model.PullRequest, error) {
	_, err := r.db.ExecContext(ctx, `
        UPDATE pull_requests
        SET status='MERGED', merged_at = COALESCE(merged_at, now())
        WHERE pull_request_id=$1 AND status='OPEN'
    `, id)
	if err != nil {
		return model.PullRequest{}, err
	}
	return r.GetWithReviewers(ctx, id)
}

func (r *PRsRepo) ListForReviewer(ctx context.Context, userID string) ([]model.PullRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status
        FROM pull_requests pr
        INNER JOIN pull_request_reviewers r ON pr.pull_request_id = r.pull_request_id
        WHERE r.user_id=$1
        ORDER BY pr.pull_request_id
    `, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []model.PullRequest
	for rows.Next() {
		var pr model.PullRequest
		if err := rows.Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status); err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prs, nil
}

func (r *PRsRepo) RemoveReviewer(ctx context.Context, prID, userID string) error {
	_, err := r.db.ExecContext(ctx, `
        DELETE FROM pull_request_reviewers
        WHERE pull_request_id=$1 AND user_id=$2
    `, prID, userID)
	return err
}

func (r *PRsRepo) AddReviewer(ctx context.Context, prID, userID string) error {
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO pull_request_reviewers(pull_request_id, user_id)
        VALUES ($1,$2)
        ON CONFLICT DO NOTHING
    `, prID, userID)
	return err
}

func (r *PRsRepo) CountAssignmentsByReviewer(ctx context.Context) ([]model.ReviewerStat, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT user_id, COUNT(*) AS assigned_count
        FROM pull_request_reviewers
        GROUP BY user_id
        ORDER BY user_id
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []model.ReviewerStat
	for rows.Next() {
		var s model.ReviewerStat
		if err := rows.Scan(&s.UserID, &s.AssignedCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}
