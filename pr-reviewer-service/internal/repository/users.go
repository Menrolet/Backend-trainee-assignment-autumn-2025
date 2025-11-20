package repository

import (
	"context"
	"database/sql"

	"pr-reviewer-service/internal/model"
)

type UsersRepo struct{ db *sql.DB }

func NewUsersRepo(db *sql.DB) *UsersRepo { return &UsersRepo{db: db} }

func (r *UsersRepo) SetIsActive(ctx context.Context, id string, active bool) (model.User, error) {
	var u model.User
	err := r.db.QueryRowContext(ctx, `
        UPDATE users
        SET is_active=$2
        WHERE user_id=$1
        RETURNING user_id, username, team_name, is_active
    `, id, active).Scan(&u.UserID, &u.Username, &u.TeamName, &u.IsActive)
	if err == sql.ErrNoRows {
		return model.User{}, ErrNotFound
	}
	return u, err
}

func (r *UsersRepo) GetUser(ctx context.Context, id string) (model.User, error) {
	var u model.User
	err := r.db.QueryRowContext(ctx, `
        SELECT user_id, username, team_name, is_active
        FROM users WHERE user_id=$1`, id).
		Scan(&u.UserID, &u.Username, &u.TeamName, &u.IsActive)
	if err == sql.ErrNoRows {
		return model.User{}, ErrNotFound
	}
	return u, err
}
