IMAGE ?= tlsbench:dev
GOFLAGS ?=
BINARY := bin/tlsbench
ROOT := $(CURDIR)

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

.PHONY: help fmt vet test test-container build run image compose-up compose-down clean

help:
	@echo "Targets:"
	@echo "  make build         Build bin/tlsbench"
	@echo "  make test          Run unit tests (requires Go 1.23+)"
	@echo "  make test-container Run unit tests in a Golang container"
	@echo "  make run           Run locally (writes ./data and ./certs)"
	@echo "  make image         Build container image ($(IMAGE))"
	@echo "  make compose-up    Run with docker compose"
	@echo "  make compose-down  Stop compose stack"
	@echo "  make fmt vet       Format and go vet"
	@echo ""
	@echo "Variables: IMAGE=$(IMAGE) GOFLAGS=$(GOFLAGS)"

fmt:
	gofmt -w ./cmd ./internal ./web

vet:
	go vet $(GOFLAGS) ./...

test:
	go test $(GOFLAGS) ./...

test-container:
	podman run --rm -v "$(ROOT)":/src -w /src docker.io/library/golang:1.23-bookworm sh /src/scripts/run-unit-tests.sh

build:
	mkdir -p bin
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BINARY) ./cmd/server

run: build
	mkdir -p "$(DATA_DIR)" "$(TLS_CA_DIR)" "$(dir $(TLS_CERT_FILE))"
	$(BINARY)

image:
	docker build -t $(IMAGE) .

compose-up:
	docker compose up --build

compose-down:
	docker compose down

clean:
	rm -rf bin coverage.out
