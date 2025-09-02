.PHONY: run test migrate init-db deps tidy build clean-mod clean

# Run the API via unified binary
run:
	./bin/xopsagent --daemon=false --config=../config/XOpsAgent.yaml

# Run all tests
test:
	go test ./...

# Run database migrations
migrate:
	@go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.0 -path db/migrations -database $$DATABASE_URL up

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
