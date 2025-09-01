package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/yourname/XOpsAgent/db/sqlc"
	"github.com/yourname/XOpsAgent/workflow"
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
	Actor    string
	IdemKey  string
}

// CreateCase inserts a new case with initial state NEW and records timeline,
// outbox and idempotency.
func (r *CaseRepository) CreateCase(ctx context.Context, args CreateCaseArgs) (db.CreateCaseRow, error) {
	if args.IdemKey != "" {
		if idem, err := r.queries.GetIdempotency(ctx, args.IdemKey); err == nil && idem.IdemKey != "" {
			var row db.CreateCaseRow
			_ = json.Unmarshal(idem.Response, &row)
			return row, nil
		}
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.CreateCaseRow{}, err
	}
	qtx := r.queries.WithTx(tx)
	caseRow, err := qtx.CreateCase(ctx, db.CreateCaseParams{
		TenantID:   args.TenantID,
		Title:      args.Title,
		Severity:   "INFO",
		Status:     string(workflow.NEW),
		ResourceID: pgtype.Int8{Valid: false},
	})
	if err != nil {
		tx.Rollback(ctx)
		return db.CreateCaseRow{}, err
	}

	// timeline
	tlPayload, _ := json.Marshal(map[string]any{"title": args.Title})
	_ = qtx.InsertTimeline(ctx, db.InsertTimelineParams{
		CaseID:  caseRow.CaseID,
		Ts:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Actor:   pgtype.Text{String: args.Actor, Valid: args.Actor != ""},
		Event:   pgtype.Text{String: "case_created", Valid: true},
		Payload: tlPayload,
	})

	// outbox
	outPayload, _ := json.Marshal(map[string]any{
		"case_id": caseRow.CaseID,
		"from":    "",
		"to":      workflow.NEW,
		"event":   "case_created",
	})
	if err := qtx.InsertOutbox(ctx, db.InsertOutboxParams{
		Aggregate:   pgtype.Text{String: "ops_case", Valid: true},
		AggregateID: pgtype.Text{String: caseRow.CaseID.String(), Valid: true},
		Topic:       pgtype.Text{String: "evt.case.transition.v1", Valid: true},
		Payload:     outPayload,
	}); err != nil {
		tx.Rollback(ctx)
		return db.CreateCaseRow{}, err
	}

	// idempotency
	respBytes, _ := json.Marshal(caseRow)
	if args.IdemKey != "" {
		ttl := pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}
		if err := qtx.InsertIdempotency(ctx, db.InsertIdempotencyParams{
			IdemKey:  args.IdemKey,
			Request:  []byte{},
			Response: respBytes,
			Ttl:      ttl,
		}); err != nil {
			tx.Rollback(ctx)
			return db.CreateCaseRow{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return db.CreateCaseRow{}, err
	}
	return caseRow, nil
}

type TransitionArgs struct {
	CaseID  pgtype.UUID
	Event   workflow.Event
	Ctx     workflow.Context
	IfMatch int64
	IdemKey string
	Request []byte
}

// Transition performs a state transition using the workflow FSM and records
// timeline, outbox and idempotency.
func (r *CaseRepository) Transition(ctx context.Context, args TransitionArgs) (db.UpdateCaseStatusRow, error) {
	if args.IdemKey != "" {
		if idem, err := r.queries.GetIdempotency(ctx, args.IdemKey); err == nil && idem.IdemKey != "" {
			var row db.UpdateCaseStatusRow
			_ = json.Unmarshal(idem.Response, &row)
			return row, nil
		}
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.UpdateCaseStatusRow{}, err
	}
	qtx := r.queries.WithTx(tx)

	current, err := qtx.GetCaseForUpdate(ctx, args.CaseID)
	if err != nil {
		tx.Rollback(ctx)
		return db.UpdateCaseStatusRow{}, err
	}
	if args.IfMatch != 0 && current.Version != args.IfMatch {
		tx.Rollback(ctx)
		return db.UpdateCaseStatusRow{}, pgx.ErrNoRows
	}

	intent, err := workflow.Decide(workflow.State(current.Status), args.Event, args.Ctx, current.CaseID.String())
	if err != nil {
		tx.Rollback(ctx)
		return db.UpdateCaseStatusRow{}, err
	}

	updated, err := qtx.UpdateCaseStatus(ctx, db.UpdateCaseStatusParams{
		CaseID:  current.CaseID,
		Status:  string(intent.To),
		Version: current.Version,
	})
	if err != nil {
		tx.Rollback(ctx)
		return db.UpdateCaseStatusRow{}, err
	}

	tlPayload, _ := json.Marshal(intent.Timeline)
	if err := qtx.InsertTimeline(ctx, db.InsertTimelineParams{
		CaseID:  current.CaseID,
		Ts:      pgtype.Timestamptz{Time: intent.Timeline.At, Valid: true},
		Actor:   pgtype.Text{String: intent.Timeline.Actor, Valid: intent.Timeline.Actor != ""},
		Event:   pgtype.Text{String: intent.Timeline.Event, Valid: true},
		Payload: tlPayload,
	}); err != nil {
		tx.Rollback(ctx)
		return db.UpdateCaseStatusRow{}, err
	}

	for _, m := range intent.Messages {
		payload, _ := json.Marshal(m.Payload)
		if err := qtx.InsertOutbox(ctx, db.InsertOutboxParams{
			Aggregate:   pgtype.Text{String: "ops_case", Valid: true},
			AggregateID: pgtype.Text{String: current.CaseID.String(), Valid: true},
			Topic:       pgtype.Text{String: m.Topic, Valid: true},
			Payload:     payload,
		}); err != nil {
			tx.Rollback(ctx)
			return db.UpdateCaseStatusRow{}, err
		}
	}

	respBytes, _ := json.Marshal(updated)
	if args.IdemKey != "" {
		ttl := pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}
		if err := qtx.InsertIdempotency(ctx, db.InsertIdempotencyParams{
			IdemKey:  args.IdemKey,
			Request:  args.Request,
			Response: respBytes,
			Ttl:      ttl,
		}); err != nil {
			tx.Rollback(ctx)
			return db.UpdateCaseStatusRow{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return db.UpdateCaseStatusRow{}, err
	}
	return updated, nil
}
