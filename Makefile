.PHONY: all build test run docker helm

GO ?= go

all: build

build:
	$(GO) build ./...

test:
	$(GO) test ./...

run:	# 本地快速起网关（其余用 compose/helm）
	go run ./cmd/gateway

docker:
	docker build -t stratolake/gateway:dev ./cmd/gateway
	docker build -t stratolake/schema-registry:dev ./cmd/schema-registry
	docker build -t stratolake/pg-writer:dev ./cmd/pg-writer
	docker build -t stratolake/ch-writer:dev ./cmd/ch-writer

helm:
	helm upgrade --install stratolake deployments/helm -n stratolake --create-namespace
