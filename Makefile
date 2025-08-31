.PHONY: build test clean run docker helm \
	    build-llm build-obs test-llm test-obs clean-llm clean-obs run-obs docker-obs helm-obs

LLM_DIR := llm-code-agent
OBS_DIR := observe-bridge

build: build-llm build-obs

test: test-llm test-obs

clean: clean-llm clean-obs

run: run-obs

docker: docker-obs

helm: helm-obs

build-llm:
	@echo "llm-code-agent: nothing to build"

test-llm:
	@echo "llm-code-agent: no tests to run"

clean-llm:
	@echo "llm-code-agent: nothing to clean"

build-obs:
	$(MAKE) -C $(OBS_DIR) build

test-obs:
	$(MAKE) -C $(OBS_DIR) test

clean-obs:
	$(MAKE) -C $(OBS_DIR) clean

run-obs:
	$(MAKE) -C $(OBS_DIR) run

docker-obs:
	$(MAKE) -C $(OBS_DIR) docker

helm-obs:
	$(MAKE) -C $(OBS_DIR) helm
