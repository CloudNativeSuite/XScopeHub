-- name: CreateCase :one
INSERT INTO ops_case (tenant_id, title, severity, status, resource_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING case_id, tenant_id, title, severity::text AS severity, status, resource_id, created_at, updated_at, labels, version;

-- name: GetCaseForUpdate :one
SELECT case_id, tenant_id, title, severity::text AS severity, status, resource_id, created_at, updated_at, labels, version
FROM ops_case
WHERE case_id = $1
FOR UPDATE;

-- name: UpdateCaseStatus :one
UPDATE ops_case
SET status = $2, updated_at = now(), version = version + 1
WHERE case_id = $1 AND version = $3
RETURNING case_id, tenant_id, title, severity::text AS severity, status, resource_id, created_at, updated_at, labels, version;

-- name: InsertTimeline :exec
INSERT INTO case_timeline (case_id, ts, actor, event, payload)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertOutbox :exec
INSERT INTO outbox (aggregate, aggregate_id, topic, payload)
VALUES ($1, $2, $3, $4);

-- name: ListUnpublishedOutbox :many
SELECT id, aggregate, aggregate_id, topic, payload, created_at
FROM outbox
WHERE published = FALSE
ORDER BY id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxPublished :exec
UPDATE outbox SET published = TRUE, published_at = now()
WHERE id = ANY($1::bigint[]);

-- name: GetIdempotency :one
SELECT idem_key, request, response, created_at, ttl
FROM idempotency
WHERE idem_key = $1;

-- name: InsertIdempotency :exec
INSERT INTO idempotency (idem_key, request, response, ttl)
VALUES ($1, $2, $3, $4);
