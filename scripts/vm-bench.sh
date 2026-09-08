#!/usr/bin/env bash
# Run from a Linux VM (or laptop) against an OpenShift Route.
# Usage: ./scripts/vm-bench.sh <base-url> [size-bytes] [curl extra args...]
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <base-url> [size-bytes] [curl args...]" >&2
  echo "example: $0 https://tlsbench-edge.apps.example.com 1048576" >&2
  echo "example: $0 https://tlsbench-passthrough.apps.example.com 1048576 -k" >&2
  echo "example: $0 https://tlsbench-passthrough.apps.example.com 1048576 --cacert ca.crt" >&2
  exit 1
fi

BASE_URL="${1%/}"
SIZE="${2:-1048576}"
shift
if [[ $# -gt 0 ]]; then
  shift
fi
CURL_EXTRA=("$@")

NAME="vm-${SIZE}.bin"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT
FILE="${WORKDIR}/${NAME}"

head -c "${SIZE}" /dev/urandom > "${FILE}"

CURL_WRITE='\nhttp_code=%{http_code} namelookup=%{time_namelookup} connect=%{time_connect} appconnect=%{time_appconnect} starttransfer=%{time_starttransfer} total=%{time_total} size_upload=%{size_upload} size_download=%{size_download}\n'

echo "== upload ${NAME} (${SIZE} bytes) to ${BASE_URL}"
curl -sS -D - -o /dev/null -w "${CURL_WRITE}" \
  --upload-file "${FILE}" \
  "${CURL_EXTRA[@]}" \
  "${BASE_URL}/api/blobs/${NAME}"

echo "== download ${NAME} from ${BASE_URL}"
curl -sS -D - -o "${WORKDIR}/download.bin" -w "${CURL_WRITE}" \
  "${CURL_EXTRA[@]}" \
  "${BASE_URL}/api/blobs/${NAME}"

echo "== info (does the pod see TLS?)"
curl -sS "${CURL_EXTRA[@]}" "${BASE_URL}/api/info"
echo
