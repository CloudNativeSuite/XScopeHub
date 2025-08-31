.PHONY: all build test run docker helm

GO ?= go

all: build

build:
	$(GO) build -o bin/observe-bridge ./cmd

test:
	$(GO) test ./...

run:	# 本地快速起网关（其余用 compose/helm）
	go run ./cmd

docker:
	docker build -t stratolake/observe-bridge:dev .

helm:
	helm upgrade --install stratolake deployments/helm -n stratolake --create-namespace
