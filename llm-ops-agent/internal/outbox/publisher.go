package outbox

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	db "github.com/yourname/XOpsAgent/db/sqlc"
)

type Publisher struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	nc       *nats.Conn
	interval time.Duration
}

func NewPublisher(pool *pgxpool.Pool, nc *nats.Conn, interval time.Duration) *Publisher {
	return &Publisher{pool: pool, queries: db.New(pool), nc: nc, interval: interval}
}

func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.flush(ctx)
		}
	}
}

func (p *Publisher) flush(ctx context.Context) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		log.Println("outbox begin tx", err)
		return
	}
	qtx := p.queries.WithTx(tx)
	rows, err := qtx.ListUnpublishedOutbox(ctx, 10)
	if err != nil {
		tx.Rollback(ctx)
		log.Println("list outbox", err)
		return
	}
	if len(rows) == 0 {
		tx.Rollback(ctx)
		return
	}
	var ids []int64
	for _, r := range rows {
		if err := p.nc.Publish(r.Topic.String, r.Payload); err != nil {
			tx.Rollback(ctx)
			log.Println("publish", err)
			return
		}
		ids = append(ids, r.ID)
	}
	if err := qtx.MarkOutboxPublished(ctx, ids); err != nil {
		tx.Rollback(ctx)
		log.Println("mark published", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Println("commit", err)
	}
	p.nc.Flush()
}
