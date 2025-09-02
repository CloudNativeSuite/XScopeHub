.PHONY: run test migrate init-db deps tidy build clean-mod clean integration-tests

# Run the API via unified binary
run: build
        ./bin/xopsagent --daemon=false --config=$${CFG:-../config/XOpsAgent.yaml}

# Run all tests
test:
	go test ./...

# Run database migrations
migrate:
	@DATABASE_URL=$${DATABASE_URL:-$$(grep -A1 "postgres:" ../config/XOpsAgent.yaml | grep url | awk '{print $$2}' | tr -d '"')} && \
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.0 -path db/migrations -database $$DATABASE_URL up

# Initialize database schema using config/XOpsAgent.yaml
init-db:
	@DB_URL=$$(grep -A1 "postgres:" ../config/XOpsAgent.yaml | grep url | awk '{print $$2}' | tr -d '"') && \
	psql $$DB_URL -f ../db/schema.sql

# Install dependencies and build
deps: tidy build

# Tidy go.mod / go.sum
tidy:
	@rm -f go.sum
	@go mod tidy

# Build project
build:
        @mkdir -p bin
        @go build -o bin/xopsagent ./cmd

# Clean build artifacts
clean:
	@rm -rf bin

# Clean and re-download modules
clean-mod:
        @go clean -modcache
        @go mod download all
        @go mod verify

# Run orchestrator integration tests
integration-tests:
        @PG_CONTAINER=llmops-agent-it-pg; \
        docker rm -f $$PG_CONTAINER >/dev/null 2>&1 || true; \
        docker run --name $$PG_CONTAINER -e POSTGRES_USER=shenlan -e POSTGRES_PASSWORD=password -e POSTGRES_DB=shenlan -p 54321:5432 -d postgres:15; \
        DATABASE_URL=postgres://shenlan:password@localhost:54321/shenlan?sslmode=disable; \
        until docker exec $$PG_CONTAINER pg_isready -U shenlan >/dev/null 2>&1; do sleep 1; done; \
        DATABASE_URL=$$DATABASE_URL make init-db; \
        DATABASE_URL=$$DATABASE_URL make migrate; \
        TMP_CFG=$$(mktemp); \
        sed "s#postgres://shenlan:password@127.0.0.1:5432/shenlan?sslmode=disable#$$DATABASE_URL#" ../config/XOpsAgent.yaml > $$TMP_CFG; \
        CFG=$$TMP_CFG make run & \
        SERVER_PID=$$!; \
        sleep 3; \
        go test ./integration-test-cases; \
        kill $$SERVER_PID; \
        rm $$TMP_CFG; \
        docker rm -f $$PG_CONTAINER >/dev/null
