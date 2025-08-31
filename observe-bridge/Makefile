.PHONY: all build test run docker helm clean

GO ?= go

all: build

build:
	$(GO) build -o bin/observe-bridge ./cmd

test:
	$(GO) test ./...

run:	# 本地快速起网关（其余用 compose/helm）
	./bin/observe-bridge --config ../config/observe-bridge-etl.yaml

docker:
	docker build -t stratolake/observe-bridge:dev .

helm:
	helm upgrade --install stratolake deployments/helm -n stratolake --create-namespace

clean:
	rm -rf bin
