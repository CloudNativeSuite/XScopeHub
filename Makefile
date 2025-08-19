.PHONY: build run test

build:
@echo "Building services"

run:
@echo "Running services"

test:
go test ./...
