IMAGE ?= tlsbench:dev
GOFLAGS ?=
BINARY := bin/tlsbench
ROOT := $(CURDIR)
GO_IMAGE ?= docker.io/library/golang:1.23-bookworm

GO := $(shell command -v go 2>/dev/null)
PODMAN := $(shell command -v podman 2>/dev/null)
DOCKER := $(shell command -v docker 2>/dev/null)

ifeq ($(PODMAN),)
  CONTAINER := $(DOCKER)
  COMPOSE := docker compose
else
  CONTAINER := $(PODMAN)
  COMPOSE := podman compose
endif

export DATA_DIR ?= $(ROOT)/data
export TLS_CERT_FILE ?= $(ROOT)/certs/tls.crt
export TLS_KEY_FILE ?= $(ROOT)/certs/tls.key
export TLS_CA_FILE ?= $(ROOT)/certs/ca.crt
export TLS_CA_DIR ?= $(ROOT)/internal-ca
export TLS_CA_BUNDLE_FILE ?= $(ROOT)/certs/ca-bundle.pem
export RESULTS_LOG ?= $(ROOT)/data/results/runs.jsonl
export TLS_DNS_NAMES ?= localhost

-include .env
export

.PHONY: help fmt vet test test-container build run image compose-up compose-down clean require-container

help:
	@echo "Targets:"
	@echo "  make test          Run unit tests (Go locally, or Podman/Docker if go is missing)"
	@echo "  make run           Run the app (local binary, or compose if go is missing)"
	@echo "  make build         Build bin/tlsbench (requires Go)"
	@echo "  make image         Build container image ($(IMAGE))"
	@echo "  make compose-up    Run with compose (ports 8080 and 8443)"
	@echo "  make compose-down  Stop compose stack"
	@echo "  make fmt vet       Format and go vet"
	@echo ""
	@echo "Go: $(if $(GO),$(GO),not found — container fallback enabled)"
	@echo "Container: $(if $(CONTAINER),$(CONTAINER),not found)"
	@echo "Variables: IMAGE=$(IMAGE) GOFLAGS=$(GOFLAGS)"

require-container:
	@if [ -z "$(CONTAINER)" ]; then \
		echo "error: Go is not installed, and neither podman nor docker was found."; \
		echo "Install Go 1.23+ (https://go.dev/dl/) or Podman/Docker, then retry."; \
		exit 1; \
	fi

fmt:
ifdef GO
	gofmt -w ./cmd ./internal ./web
else
	@$(MAKE) require-container
	$(CONTAINER) run --rm -v "$(ROOT)":/src -w /src $(GO_IMAGE) gofmt -w ./cmd ./internal ./web
endif

vet:
ifdef GO
	go vet $(GOFLAGS) ./...
else
	@$(MAKE) require-container
	$(CONTAINER) run --rm -v "$(ROOT)":/src -w /src $(GO_IMAGE) go vet ./...
endif

test:
ifdef GO
	go test $(GOFLAGS) ./...
else
	@echo "Go not found on PATH; running tests with $(CONTAINER)"
	@$(MAKE) test-container
endif

test-container: require-container
	$(CONTAINER) run --rm -v "$(ROOT)":/src -w /src $(GO_IMAGE) sh /src/scripts/run-unit-tests.sh

build:
ifdef GO
	mkdir -p bin
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BINARY) ./cmd/server
else
	@echo "Go not found on PATH; a macOS/Windows host cannot run a Linux binary from the container."
	@echo "Use: make run    (starts the app with compose)"
	@echo "Or:  make image"
	@exit 1
endif

run:
ifdef GO
	@$(MAKE) build
	mkdir -p "$(DATA_DIR)" "$(TLS_CA_DIR)" "$(dir $(TLS_CERT_FILE))"
	$(BINARY)
else
	@echo "Go not found on PATH; starting with $(COMPOSE)"
	@$(MAKE) compose-up
endif

image: require-container
	$(CONTAINER) build -t $(IMAGE) .

compose-up: require-container
	$(COMPOSE) up --build

compose-down: require-container
	$(COMPOSE) down

clean:
	rm -rf bin coverage.out
