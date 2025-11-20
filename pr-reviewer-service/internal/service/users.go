package service

import (
	"context"

	"pr-reviewer-service/internal/model"
	"pr-reviewer-service/internal/repository"
)

type UsersService struct {
	users *repository.UsersRepo
}

func NewUsersService(users *repository.UsersRepo) *UsersService {
	return &UsersService{users: users}
}

func (s *UsersService) SetIsActive(ctx context.Context, id string, active bool) (model.User, error) {
	return s.users.SetIsActive(ctx, id, active)
}

func (s *UsersService) Get(ctx context.Context, id string) (model.User, error) {
	return s.users.GetUser(ctx, id)
}
