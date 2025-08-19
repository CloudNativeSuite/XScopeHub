package graph

import (
	"context"
	"database/sql"
	"fmt"
)

// DAO wraps access to an Apache AGE graph in PostgreSQL.
type DAO struct {
	db        *sql.DB
	graphName string
}

// NewDAO creates a new graph DAO.
func NewDAO(db *sql.DB, graphName string) *DAO {
	return &DAO{db: db, graphName: graphName}
}

// CreateCallEdge ensures a CALLS edge between two services exists.
func (d *DAO) CreateCallEdge(ctx context.Context, from, to string) error {
	query := fmt.Sprintf("SELECT * FROM cypher('%s', $$ MERGE (a:Service {name: $1}) MERGE (b:Service {name: $2}) MERGE (a)-[:CALLS]->(b) $$) AS (a agtype, b agtype)", d.graphName)
	_, err := d.db.ExecContext(ctx, query, from, to)
	return err
}

// ServiceDependencies returns dependencies within k hops from a service.
func (d *DAO) ServiceDependencies(ctx context.Context, svc string, k int) (*sql.Rows, error) {
	query := fmt.Sprintf("SELECT * FROM cypher('%s', $$ MATCH p = (s:Service {name: $1})-[:CALLS*1..%d]->(t:Service) RETURN p $$) AS (p agtype)", d.graphName, k)
	return d.db.QueryContext(ctx, query, svc)
}
