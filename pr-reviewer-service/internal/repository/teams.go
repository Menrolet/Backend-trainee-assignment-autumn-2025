package repository

import (
	"context"
	"database/sql"
	"errors"

	"pr-reviewer-service/internal/model"
)

var ErrTeamExists = errors.New("team exists")
var ErrNotFound = errors.New("not found")

type TeamsRepo struct {
	db *sql.DB
}

func NewTeamsRepo(db *sql.DB) *TeamsRepo { return &TeamsRepo{db: db} }

func (r *TeamsRepo) CreateTeamWithMembers(ctx context.Context, t model.Team) (model.Team, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Team{}, err
	}
	defer tx.Rollback()

	var tmp string
	err = tx.QueryRowContext(ctx, `SELECT team_name FROM teams WHERE team_name=$1`, t.TeamName).
		Scan(&tmp)
	if err == nil {
		return model.Team{}, ErrTeamExists
	}
	if err != nil && err != sql.ErrNoRows {
		return model.Team{}, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO teams(team_name) VALUES ($1)`, t.TeamName); err != nil {
		return model.Team{}, err
	}

	for _, m := range t.Members {
		_, err := tx.ExecContext(ctx, `
            INSERT INTO users (user_id, username, team_name, is_active)
            VALUES ($1,$2,$3,$4)
            ON CONFLICT (user_id) DO UPDATE
              SET username = EXCLUDED.username,
                  team_name = EXCLUDED.team_name,
                  is_active = EXCLUDED.is_active
        `, m.UserID, m.Username, t.TeamName, m.IsActive)
		if err != nil {
			return model.Team{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return model.Team{}, err
	}
	return t, nil
}

func (r *TeamsRepo) GetTeam(ctx context.Context, name string) (model.Team, error) {
	var t model.Team
	t.TeamName = name

	row := r.db.QueryRowContext(ctx, `SELECT team_name FROM teams WHERE team_name=$1`, name)
	if err := row.Scan(&t.TeamName); err != nil {
		if err == sql.ErrNoRows {
			return model.Team{}, ErrNotFound
		}
		return model.Team{}, err
	}

	rows, err := r.db.QueryContext(ctx, `
        SELECT user_id, username, is_active
        FROM users
        WHERE team_name=$1
        ORDER BY user_id
    `, name)
	if err != nil {
		return model.Team{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var m model.TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return model.Team{}, err
		}
		t.Members = append(t.Members, m)
	}
	return t, nil
}
