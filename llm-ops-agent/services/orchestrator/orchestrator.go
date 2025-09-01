package orchestrator

import (
    "context"

    db "github.com/yourname/XOpsAgent/db/sqlc"
    "github.com/yourname/XOpsAgent/ports"
)

type Service interface {
    CreateCase(ctx context.Context, args ports.CreateCaseArgs) (db.CreateCaseRow, error)
    Transition(ctx context.Context, args ports.TransitionArgs) (db.UpdateCaseStatusRow, error)
}

type service struct {
    repo ports.CaseRepository
}

func New(repo ports.CaseRepository) Service {
    return &service{repo: repo}
}

func (s *service) CreateCase(ctx context.Context, args ports.CreateCaseArgs) (db.CreateCaseRow, error) {
    return s.repo.CreateCase(ctx, args)
}

func (s *service) Transition(ctx context.Context, args ports.TransitionArgs) (db.UpdateCaseStatusRow, error) {
    return s.repo.Transition(ctx, args)
}

