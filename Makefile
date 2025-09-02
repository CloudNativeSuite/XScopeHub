.PHONY: build test clean run docker helm \
            build-llm build-obs test-llm test-obs clean-llm clean-obs run-obs docker-obs helm-obs \
            integration-tests integration-tests-llm integration-tests-obs

LLM_DIR := llm-ops-agent
OBS_DIR := observe-bridge

build: build-llm build-obs

test: test-llm test-obs

clean: clean-llm clean-obs

run: run-obs run-llm-ops-agent

docker: docker-obs

helm: helm-obs

build-llm:
	$(MAKE) -C $(LLM_DIR) build

test-llm:
	$(MAKE) -C $(LLM_DIR) test

clean-llm:
	$(MAKE) -C $(LLM_DIR) clean

build-obs:
	$(MAKE) -C $(OBS_DIR) build

test-obs:
	$(MAKE) -C $(OBS_DIR) test

clean-obs:
	$(MAKE) -C $(OBS_DIR) clean

run-obs:
	$(MAKE) -C $(OBS_DIR) run
run-llm-ops-agent:
	$(MAKE) -C $(LLM_DIR) run

docker-obs:
	$(MAKE) -C $(OBS_DIR) docker

helm-obs:
	$(MAKE) -C $(OBS_DIR) helm

integration-tests: integration-tests-llm integration-tests-obs

integration-tests-llm:
	$(MAKE) -C $(LLM_DIR) integration-tests

integration-tests-obs:
	$(MAKE) -C $(OBS_DIR) integration-tests
