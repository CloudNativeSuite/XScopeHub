CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SELECT * FROM create_graph('xinsight');
-- Nodes and edges are created dynamically via application logic.

