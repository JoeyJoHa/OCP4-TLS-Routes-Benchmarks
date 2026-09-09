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
CURL_EXTRA=()
if [[ $# -gt 0 ]]; then
  CURL_EXTRA=("$@")
fi

NAME="vm-${SIZE}.bin"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT
FILE="${WORKDIR}/${NAME}"

head -c "${SIZE}" /dev/urandom > "${FILE}"

CURL_JSON='{"time_namelookup":%{time_namelookup},"time_connect":%{time_connect},"time_appconnect":%{time_appconnect},"time_pretransfer":%{time_pretransfer},"time_starttransfer":%{time_starttransfer},"time_redirect":%{time_redirect},"time_total":%{time_total},"http_code":%{http_code}}'

# Empty arrays are "unbound" under `set -u` on Bash 3.2 (macOS).
curl_extra() {
  curl -sS ${CURL_EXTRA[@]+"${CURL_EXTRA[@]}"} "$@"
}

submit_timings() {
  local operation="$1"
  local payload="$2"
  local inner="${payload#\{}"
  inner="${inner%\}}"
  curl_extra \
    -H 'Content-Type: application/json' \
    --data "{\"operation\":\"${operation}\",\"name\":\"${NAME}\",${inner}}" \
    "${BASE_URL}/api/results/timings" >/dev/null
}

echo "== upload ${NAME} (${SIZE} bytes) to ${BASE_URL}"
UPLOAD_TIMING="$(
  curl_extra -o "${WORKDIR}/upload.json" -w "${CURL_JSON}" \
    --upload-file "${FILE}" \
    "${BASE_URL}/api/blobs/${NAME}"
)"
cat "${WORKDIR}/upload.json"
echo
echo "curl ${UPLOAD_TIMING}"
submit_timings upload "${UPLOAD_TIMING}"

echo "== download ${NAME} from ${BASE_URL}"
DOWNLOAD_TIMING="$(
  curl_extra -o "${WORKDIR}/download.bin" -w "${CURL_JSON}" \
    "${BASE_URL}/api/blobs/${NAME}"
)"
echo "curl ${DOWNLOAD_TIMING}"
submit_timings download "${DOWNLOAD_TIMING}"

echo "== info (does the pod see TLS?)"
curl_extra "${BASE_URL}/api/info"
echo
