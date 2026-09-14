#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
gofmt -w ./cmd ./internal ./web
exec go test ./...
