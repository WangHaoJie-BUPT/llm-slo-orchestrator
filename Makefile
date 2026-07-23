GO ?= go
PYTHON ?= python3
BIN_DIR ?= bin
COMPOSE_FILE ?= deploy/compose/compose.yaml
MODEL_DIR ?= /path/to/Qwen3-0.6B-c1899de
VLLM_BASE_URL ?= http://127.0.0.1:8000
W1_EVIDENCE_DIR ?= evidence/w1-$(shell date +%Y%m%dT%H%M%S%z)
LOADGEN_PYTHON ?= .venv/bin/python
W1_BENCHMARK_EVIDENCE_DIR ?= evidence/w1-capacity-$(shell date +%Y%m%dT%H%M%S%z)
W2_FAKE_EVIDENCE_DIR ?= evidence/w2-fake-$(shell date +%Y%m%dT%H%M%S%z)
W2_GPU_EVIDENCE_DIR ?= evidence/w2-router-gpu-$(shell date +%Y%m%dT%H%M%S%z)
GO_FILES := $(shell find cmd internal -type f -name '*.go')

.PHONY: fmt fmt-check lint test test-race build verify verify-versions verify-layout
.PHONY: compose-config compose-up compose-down dashboard-up gpu-up gpu-down verify-model verify-loadgen
.PHONY: e2e-fake e2e-gpu e2e-router-gpu benchmark benchmark-w1 report
.PHONY: fault-smoke kind-up kind-down cloud-clean-check clean

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@files="$$(gofmt -l $(GO_FILES))"; if [ -n "$$files" ]; then echo "gofmt required:"; echo "$$files"; exit 1; fi

lint:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -buildvcs=false -trimpath -o $(BIN_DIR)/router ./cmd/router
	$(GO) build -buildvcs=false -trimpath -o $(BIN_DIR)/profile-controller ./cmd/profile-controller
	$(GO) build -buildvcs=false -trimpath -o $(BIN_DIR)/fake-backend ./cmd/fake-backend

verify-versions:
	$(PYTHON) hack/verify_versions.py

verify-layout:
	$(PYTHON) hack/verify_layout.py

verify: fmt-check lint test test-race build verify-versions verify-layout

compose-config:
	MODEL_DIR=$(MODEL_DIR) docker compose -f $(COMPOSE_FILE) --profile observability --profile gpu config --quiet

compose-up:
	docker compose -f deploy/compose/compose.yaml --profile observability up -d

compose-down:
	docker compose -f deploy/compose/compose.yaml --profile observability down

dashboard-up: compose-up

verify-model:
	$(PYTHON) hack/verify_model.py --model-dir $(MODEL_DIR)

gpu-up: verify-model
	MODEL_DIR=$(MODEL_DIR) docker compose -f $(COMPOSE_FILE) --profile gpu up -d --wait --wait-timeout 900 vllm

gpu-down:
	MODEL_DIR=$(MODEL_DIR) docker compose -f $(COMPOSE_FILE) --profile gpu down

e2e-gpu: gpu-up
	$(PYTHON) hack/w1_smoke.py --base-url $(VLLM_BASE_URL) --model qwen3-0.6b --evidence-dir $(W1_EVIDENCE_DIR)

e2e-fake: build
	$(PYTHON) hack/e2e_fake.py --router $(BIN_DIR)/router --backend $(BIN_DIR)/fake-backend --evidence-dir $(W2_FAKE_EVIDENCE_DIR)

e2e-router-gpu: gpu-up build
	$(PYTHON) hack/e2e_router_gpu.py --router $(BIN_DIR)/router --backend-url $(VLLM_BASE_URL) --evidence-dir $(W2_GPU_EVIDENCE_DIR)

verify-loadgen:
	$(LOADGEN_PYTHON) hack/verify_loadgen.py

benchmark: benchmark-w1

benchmark-w1: gpu-up verify-loadgen
	$(LOADGEN_PYTHON) hack/w1_guidellm.py --base-url $(VLLM_BASE_URL) --scenario experiments/capacity.yaml --evidence-dir $(W1_BENCHMARK_EVIDENCE_DIR)

report fault-smoke kind-up kind-down cloud-clean-check:
	$(PYTHON) hack/not_implemented.py $@

clean:
	$(GO) clean
	$(PYTHON) -c "import pathlib, shutil; shutil.rmtree(pathlib.Path('$(BIN_DIR)'), ignore_errors=True)"
