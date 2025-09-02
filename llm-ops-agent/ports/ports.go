package ports

import (
    "context"

    db "github.com/yourname/XOpsAgent/db/sqlc"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/yourname/XOpsAgent/workflow"
)

type CreateCaseArgs struct {
    TenantID int64
    Title    string
    Actor    string
    IdemKey  string
}

type TransitionArgs struct {
    CaseID  pgtype.UUID
    Event   workflow.Event
    Ctx     workflow.Context
    IfMatch int64
    IdemKey string
    Request []byte
}

type CaseRepository interface {
    CreateCase(ctx context.Context, args CreateCaseArgs) (db.CreateCaseRow, error)
    Transition(ctx context.Context, args TransitionArgs) (db.UpdateCaseStatusRow, error)
}

