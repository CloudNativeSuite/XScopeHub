-- name: CreateCase :one
INSERT INTO ops_case (tenant_id, title, severity, status, resource_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING case_id, tenant_id, title, severity::text AS severity, status, resource_id, created_at, updated_at, labels;

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
