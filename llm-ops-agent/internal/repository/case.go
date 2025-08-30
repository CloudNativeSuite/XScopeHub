package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/yourname/XOpsAgent/db/sqlc"
)

type CaseRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewCaseRepository(pool *pgxpool.Pool) *CaseRepository {
	return &CaseRepository{pool: pool, queries: db.New(pool)}
}

type CreateCaseArgs struct {
	TenantID int64
	Title    string
}

func (r *CaseRepository) CreateCase(ctx context.Context, args CreateCaseArgs) (db.CreateCaseRow, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.CreateCaseRow{}, err
	}
	qtx := r.queries.WithTx(tx)
	caseRow, err := qtx.CreateCase(ctx, db.CreateCaseParams{
		TenantID:   args.TenantID,
		Title:      args.Title,
		Severity:   "INFO",
		Status:     "NEW",
		ResourceID: pgtype.Int8{Valid: false},
	})
	if err != nil {
		tx.Rollback(ctx)
		return db.CreateCaseRow{}, err
	}
	payload, _ := json.Marshal(map[string]any{"case_id": caseRow.CaseID})
	err = qtx.InsertOutbox(ctx, db.InsertOutboxParams{
		Aggregate:   pgtype.Text{String: "ops_case", Valid: true},
		AggregateID: pgtype.Text{String: caseRow.CaseID.String(), Valid: true},
		Topic:       pgtype.Text{String: "evt.case.created.v1", Valid: true},
		Payload:     payload,
	})
	if err != nil {
		tx.Rollback(ctx)
		return db.CreateCaseRow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.CreateCaseRow{}, err
	}
	return caseRow, nil
}
