# PostgreSQL Initialization

This document describes how to set up the PostgreSQL schema used by the XOps Agent. The SQL script `db/init.sql` creates all tables, indexes and extensions needed by the agent. The instructions apply to both Linux and macOS.

## Prerequisites

- PostgreSQL 15+ with superuser access.
- Extensions: `timescaledb`, `pg_trgm`, `pgcrypto`, `vector`, `age`, `btree_gist`.

## Initialization Steps

1. Start PostgreSQL and create a database, e.g. `ops`.
2. Run the initialization script:

   ```bash
   psql -d ops -f db/init.sql
   ```

   The script enables required extensions and creates all tables.

### Linux

Install PostgreSQL and extensions via apt:

```bash
sudo apt-get install postgresql postgresql-contrib timescaledb-postgresql-15
```

Then start the service and run the initialization script above.

### macOS

Install PostgreSQL and TimescaleDB with Homebrew:

```bash
brew install postgresql@15 timescaledb
brew services start postgresql@15
```

Load the TimescaleDB extension once:

```bash
psql -d postgres -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"
```

Then execute `db/init.sql` against your database.
