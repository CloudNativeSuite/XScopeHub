package vector

import (
	"context"
	"database/sql"
)

// SemanticObject represents a row in the semantic_objects table.
type SemanticObject struct {
	ID         string
	ObjectType string
	Service    sql.NullString
	Host       sql.NullString
	TraceID    sql.NullString
	SpanID     sql.NullString
	Timestamp  sql.NullTime
	Title      sql.NullString
	Content    string
	Labels     []byte
	Embedding  []float32
	RefSource  sql.NullString
	RefKey     []byte
}

// DAO provides access to semantic object storage backed by PostgreSQL with pgvector.
type DAO struct {
	db *sql.DB
}

// NewDAO creates a new DAO.
func NewDAO(db *sql.DB) *DAO {
	return &DAO{db: db}
}

// Insert inserts a semantic object into the database.
func (d *DAO) Insert(ctx context.Context, obj *SemanticObject) error {
	_, err := d.db.ExecContext(ctx, `
        INSERT INTO semantic_objects (
            object_type, service, host, trace_id, span_id, ts,
            title, content, labels, embedding, ref_source, ref_key
        ) VALUES (
            $1, $2, $3, $4, $5, $6,
            $7, $8, $9, $10, $11, $12
        )`,
		obj.ObjectType, obj.Service, obj.Host, obj.TraceID, obj.SpanID, obj.Timestamp,
		obj.Title, obj.Content, obj.Labels, obj.Embedding, obj.RefSource, obj.RefKey,
	)
	return err
}

// SearchTopK performs a simple Top-K similarity search using pgvector.
func (d *DAO) SearchTopK(ctx context.Context, service string, queryVec []float32, k int) (*sql.Rows, error) {
	const q = `
        SELECT id, object_type, service, ts, title, content,
               1 - (embedding <=> $1) AS score
        FROM semantic_objects
        WHERE service = $2
          AND ts >= now() - interval '7 days'
        ORDER BY embedding <=> $1
        LIMIT $3
    `
	return d.db.QueryContext(ctx, q, queryVec, service, k)
}

// SearchHybrid demonstrates a hybrid search combining vector and full text scores.
func (d *DAO) SearchHybrid(ctx context.Context, queryVec []float32, queryText string, k int) (*sql.Rows, error) {
	const q = `
        WITH v AS (
            SELECT id, 1 - (embedding <=> $1) AS vscore
            FROM semantic_objects
            WHERE ts >= now() - interval '30 days'
            ORDER BY embedding <=> $1
            LIMIT 200
        )
        SELECT s.id, s.object_type, s.service, s.ts, s.title, s.content,
               v.vscore,
               ts_rank(s.fts, plainto_tsquery($2)) AS tscore
        FROM v JOIN semantic_objects s USING(id)
        ORDER BY (vscore * 0.7 + tscore * 0.3) DESC
        LIMIT $3
    `
	return d.db.QueryContext(ctx, q, queryVec, queryText, k)
}
