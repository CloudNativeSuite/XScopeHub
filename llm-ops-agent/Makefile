.PHONY: run test migrate deps tidy build clean-mod clean

# Run the API via unified binary
run:
	go run ./cmd --daemon=false

# Run all tests
test:
	go test ./...

# Run database migrations
migrate:
	migrate -path internal/migrations -database $$DATABASE_URL up

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
