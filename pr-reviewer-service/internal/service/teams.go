package service

import (
	"context"

	"pr-reviewer-service/internal/model"
	"pr-reviewer-service/internal/repository"
)

type TeamsService struct {
	teams *repository.TeamsRepo
}

func NewTeamsService(teams *repository.TeamsRepo) *TeamsService {
	return &TeamsService{teams: teams}
}

func (s *TeamsService) Create(ctx context.Context, t model.Team) (model.Team, error) {
	return s.teams.CreateTeamWithMembers(ctx, t)
}

func (s *TeamsService) Get(ctx context.Context, name string) (model.Team, error) {
	return s.teams.GetTeam(ctx, name)
}
