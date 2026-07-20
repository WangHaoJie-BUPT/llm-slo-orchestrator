GO ?= go
PYTHON ?= python3
BIN_DIR ?= bin
GO_FILES := $(shell find cmd internal -type f -name '*.go')

.PHONY: fmt fmt-check lint test test-race build verify verify-versions verify-layout
.PHONY: compose-config compose-up compose-down dashboard-up e2e-fake e2e-gpu benchmark report
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

verify-versions:
	$(PYTHON) hack/verify_versions.py

verify-layout:
	$(PYTHON) hack/verify_layout.py

verify: fmt-check lint test test-race build verify-versions verify-layout

compose-config:
	docker compose -f deploy/compose/compose.yaml --profile observability config --quiet

compose-up:
	docker compose -f deploy/compose/compose.yaml --profile observability up -d

compose-down:
	docker compose -f deploy/compose/compose.yaml --profile observability down

dashboard-up: compose-up

e2e-fake e2e-gpu benchmark report fault-smoke kind-up kind-down cloud-clean-check:
	$(PYTHON) hack/not_implemented.py $@

clean:
	$(GO) clean
	$(PYTHON) -c "import pathlib, shutil; shutil.rmtree(pathlib.Path('$(BIN_DIR)'), ignore_errors=True)"
