# Testing

This guide describes how to run the test suite and verify the service locally.

## Run unit tests

```bash
make test
```

## Start dependent services and initialize the database

Start TimescaleDB, NATS, Redpanda and the OTel Collector:

```bash
docker compose up -d
```

Run database migrations (requires `migrate` with the Postgres driver):

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/ops?sslmode=disable"
make migrate
```

## Launch the service

```bash
make run
```

or build and run the container:

```bash
docker compose up api -d --build
```

## Verify endpoints

The service exposes a health check and Prometheus metrics:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

A healthy instance responds with `{ "status": "ok" }` at `/healthz` and standard metrics at `/metrics`.

