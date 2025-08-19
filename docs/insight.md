# XInsight

XInsight combines vector similarity search with graph analytics to assist incident
response. PostgreSQL with pgvector stores semantic objects such as log segments
or knowledge base fragments, while Apache AGE maintains service dependency
relationships.

## Schema

Migration `0003_pgvector_semantic_objects.sql` creates the `semantic_objects`
table and its indices. `0004_age_init.sql` initializes the AGE graph named
`xinsight`.

## Queries

### Top-K Similarity

```sql
SELECT id, object_type, service, ts, title,
       1 - (embedding <=> :qvec) AS score
FROM semantic_objects
WHERE object_type IN ('alert', 'log', 'doc')
ORDER BY embedding <=> :qvec
LIMIT 20;
```

### Service Dependency

```sql
SELECT * FROM cypher('xinsight', $$
  MATCH p = (s:Service {name: $svc})-[:CALLS*1..3]->(t:Service)
  RETURN p
$$) AS (p agtype);
```

These building blocks allow implementing a search → discovery → root cause
analysis workflow across metrics, logs and traces.
