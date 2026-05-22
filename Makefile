BINARY      := es-sampler
BIN_DIR     := bin
PKG         := ./...
CMD         := .
GO          ?= go
GOFMT       ?= gofmt
GO_FILES    := $(shell find . -type f -name '*.go' -not -path './.git/*')

IMAGE_REGISTRY  ?= ghcr.io
IMAGE_NAMESPACE ?= ruflin
IMAGE_NAME      ?= es-sampler
IMAGE_TAG       ?= dev
IMAGE           ?= $(IMAGE_REGISTRY)/$(IMAGE_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_TAG)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the binary into $(BIN_DIR)/$(BINARY)
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(BINARY) $(CMD)

.PHONY: run
run: ## Run the CLI (pass ARGS="--help" to forward flags)
	$(GO) run $(CMD) $(ARGS)

.PHONY: test
test: ## Run unit tests
	$(GO) test -race $(PKG)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: fmt
fmt: ## Format code with gofmt -w
	$(GOFMT) -w $(GO_FILES)

.PHONY: fmt-check
fmt-check: ## Fail if any file is not gofmt-clean
	@diff=$$($(GOFMT) -l $(GO_FILES)); \
	if [ -n "$$diff" ]; then \
		echo "gofmt found unformatted files:"; \
		echo "$$diff"; \
		echo "run 'make fmt' to fix"; \
		exit 1; \
	fi

.PHONY: lint
lint: vet fmt-check ## Run static checks (vet + gofmt)

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

.PHONY: tidy-check
tidy-check: ## Fail if go.mod / go.sum would be changed by tidy
	@cp go.mod go.mod.bak; cp go.sum go.sum.bak; \
	$(GO) mod tidy; \
	if ! diff -q go.mod go.mod.bak > /dev/null || ! diff -q go.sum go.sum.bak > /dev/null; then \
		mv go.mod.bak go.mod; mv go.sum.bak go.sum; \
		echo "go.mod / go.sum are not tidy; run 'make tidy'"; \
		exit 1; \
	fi; \
	rm go.mod.bak go.sum.bak

.PHONY: check
check: lint test build ## Run lint + test + build (CI entry point)

.PHONY: image
image: ## Build the container image (override IMAGE=... or IMAGE_TAG=... to retag)
	docker build \
	  --build-arg BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
	  --build-arg SOURCE_COMMIT=$$(git rev-parse HEAD) \
	  --build-arg VERSION=$(IMAGE_TAG) \
	  -t $(IMAGE) .

.PHONY: push
push: image ## Build and push the image (must be logged in to $(IMAGE_REGISTRY) first)
	docker push $(IMAGE)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	$(GO) clean -testcache
