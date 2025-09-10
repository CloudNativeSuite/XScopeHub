.PHONY: all build test run docker helm clean init-db migrate integration-tests test-oo-stream

GO ?= go
PG_URL ?= $(DATABASE_URL)

all: build

build:
	$(GO) build -o bin/observe-bridge ./cmd

test:
	$(GO) test ./...
	$(MAKE) test-oo-stream

test-oo-stream:
	$(GO) test ./integration-test-cases -run TestOOStream -count=1

run:    # 本地快速起网关（其余用 compose/helm）
	./bin/observe-bridge --config ../config/observe-bridge-etl.yaml

docker:
	docker build -t stratolake/observe-bridge:dev .

helm:
	helm upgrade --install stratolake deployments/helm -n stratolake --create-namespace

init-db:
	psql $(PG_URL) -c 'SELECT 1'

migrate:
	$(GO) install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	migrate -path migrations/postgres -database $(PG_URL) up

integration-tests: build
	$(MAKE) init-db
	$(MAKE) migrate
	envsubst < integration-test-cases/config.template.yaml > integration-test-cases/config.yaml
	./bin/observe-bridge --config integration-test-cases/config.yaml &
	PID=$$!
	sleep 2
	go test ./integration-test-cases -count=1
	kill $$PID
	rm integration-test-cases/config.yaml

clean:
	rm -rf bin
