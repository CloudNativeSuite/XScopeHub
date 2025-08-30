# Deployment

## Quick Start (PoC)

1. Clone project
   ```bash
   git clone https://github.com/your-org/xscopehub.git
   cd xscopehub/observe-bridge
   ```
2. Prepare environment variables
   ```bash
   cp .env.example .env
   # Required example (modify as needed)
   # PG_PASSWORD=changeme
   # CH_USER=default
   # CH_PASSWORD=
   # GRAFANA_ADMIN_PASSWORD=admin
   ```
3. One-click start (Docker Compose)
   ```bash
   cd deployments/docker-compose
   docker compose -f poc.yaml up -d
   ```
4. Verify services
   - Gateway API: http://localhost:8080/health
   - Grafana: http://localhost:3000 (default admin/admin or use your environment variables)
   - PostgreSQL: localhost:5432
   - ClickHouse: http://localhost:8123/ping
5. Collector example (Vector → Gateway)
   `agents/vector/vector.toml`:
   ```toml
   [sources.prom]
   type = "prometheus_scrape"
   endpoints = ["http://localhost:9100/metrics"]

   [sources.logs]
   type = "journald"

   [transforms.jsonify_logs]
   type = "remap"
   inputs = ["logs"]
   source = '''
   .structured = parse_json!(string!(.message))
   '''

   [sinks.gateway_metrics]
   type = "http"
   inputs = ["prom"]
   uri = "http://localhost:8080/write/metrics"
   encoding.codec = "json"

   [sinks.gateway_logs]
   type = "http"
   inputs = ["jsonify_logs"]
   uri = "http://localhost:8080/write/logs"
   encoding.codec = "json"
   ```

