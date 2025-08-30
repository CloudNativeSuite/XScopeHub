package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/yourname/XOpsAgent/internal/outbox"
	"github.com/yourname/XOpsAgent/internal/repository"
)

func main() {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/ops?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Drain()

	pub := outbox.NewPublisher(pool, nc, time.Second)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go pub.Run(runCtx)

	done := make(chan struct{})
	_, err = nc.Subscribe("evt.case.created.v1", func(m *nats.Msg) {
		fmt.Printf("received: %s\n", string(m.Data))
		done <- struct{}{}
	})
	if err != nil {
		log.Fatal(err)
	}

	repo := repository.NewCaseRepository(pool)
	if _, err := repo.CreateCase(ctx, repository.CreateCaseArgs{TenantID: 1, Title: "demo"}); err != nil {
		log.Fatal(err)
	}

	<-done
	time.Sleep(time.Second)
	cancel()
	<-runCtx.Done()
}
