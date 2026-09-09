#!/usr/bin/env bash
# Run from a Linux VM (or laptop) against an OpenShift Route or local tlsbench.
#
# Usage:
#   ./scripts/vm-bench.sh [options] <base-url> [size-bytes] [curl args...]
#
# Options:
#   --repeat N        Samples after warmup (default: 1 for blob, 30 for --handshake-only)
#   --warmup N        Discarded warmup samples (default: 3 when --repeat > 1)
#   --handshake-only  GET /api/bench/probe only (no blob I/O)
#   --http1.1         Force curl --http1.1 (pin ALPN)
#   --reuse           Reuse TCP/TLS connection across samples (default: cold per sample)
#   --route-mode M    Label rows: edge|passthrough|reencrypt|service-http|service-https
#   --experiment-id ID  Group samples (default: auto-generated)
set -euo pipefail

REPEAT=0
WARMUP=0
HANDSHAKE_ONLY=false
HTTP11=false
REUSE=false
ROUTE_MODE=""
EXPERIMENT_ID=""

usage() {
  echo "usage: $0 [options] <base-url> [size-bytes] [curl args...]" >&2
  echo "example: $0 https://tlsbench-passthrough.apps.example.com 1048576 --cacert ca.crt" >&2
  echo "example: $0 --handshake-only --http1.1 --route-mode passthrough https://127.0.0.1:8443 -k" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repeat)
      REPEAT="${2:-}"
      shift 2
      ;;
    --warmup)
      WARMUP="${2:-}"
      shift 2
      ;;
    --handshake-only)
      HANDSHAKE_ONLY=true
      shift
      ;;
    --http1.1)
      HTTP11=true
      shift
      ;;
    --reuse)
      REUSE=true
      shift
      ;;
    --route-mode)
      ROUTE_MODE="${2:-}"
      shift 2
      ;;
    --experiment-id)
      EXPERIMENT_ID="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -lt 1 ]]; then
  usage
fi

BASE_URL="${1%/}"
SIZE="${2:-1048576}"
shift || true
if [[ $# -gt 0 ]]; then
  shift || true
fi
CURL_EXTRA=()
if [[ $# -gt 0 ]]; then
  CURL_EXTRA=("$@")
fi

if [[ "$HANDSHAKE_ONLY" == true ]]; then
  if [[ "$REPEAT" -eq 0 ]]; then
    REPEAT=30
  fi
  if [[ "$WARMUP" -eq 0 ]]; then
    WARMUP=3
  fi
else
  if [[ "$REPEAT" -eq 0 ]]; then
    REPEAT=1
  fi
  if [[ "$WARMUP" -eq 0 && "$REPEAT" -gt 1 ]]; then
    WARMUP=3
  fi
fi

if [[ -z "$EXPERIMENT_ID" ]]; then
  EXPERIMENT_ID="vm-$(date +%s)-$$"
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

CURL_JSON='{"time_namelookup":%{time_namelookup},"time_connect":%{time_connect},"time_appconnect":%{time_appconnect},"time_pretransfer":%{time_pretransfer},"time_starttransfer":%{time_starttransfer},"time_redirect":%{time_redirect},"time_total":%{time_total},"http_code":%{http_code}}'

bench_headers=()
if [[ -n "$ROUTE_MODE" ]]; then
  bench_headers=(-H "X-Route-Mode: ${ROUTE_MODE}")
fi

curl_extra() {
  curl -sS ${CURL_EXTRA[@]+"${CURL_EXTRA[@]}"} "$@"
}

curl_protocol_flags=()
if [[ "$HTTP11" == true ]]; then
  curl_protocol_flags=(--http1.1)
fi

curl_conn_flags=()
if [[ "$REUSE" != true ]]; then
  curl_conn_flags=(--no-keepalive)
fi

declare -a TLS_HS_SAMPLES=()
declare -a TCP_SAMPLES=()
declare -a TOTAL_SAMPLES=()

percentile_summary() {
  python3 - "$@" <<'PY'
import json, sys
values = [float(v) for v in sys.argv[1:] if float(v) > 0]
if not values:
    print("{}")
    sys.exit(0)
values.sort()
def pct(p):
    if len(values) == 1:
        return values[0]
    rank = (p / 100) * (len(values) - 1)
    lo = int(rank)
    hi = min(lo + 1, len(values) - 1)
    w = rank - lo
    return values[lo] * (1 - w) + values[hi] * w
summary = {
    "count": len(values),
    "min_ms": round(values[0], 3),
    "max_ms": round(values[-1], 3),
    "mean_ms": round(sum(values) / len(values), 3),
    "p50_ms": round(pct(50), 3),
    "p90_ms": round(pct(90), 3),
    "p99_ms": round(pct(99), 3),
}
print(json.dumps(summary))
PY
}

curl_phases_ms() {
  python3 -c "
import json, sys
d = json.loads(sys.argv[1])
lookup = d.get('time_namelookup', 0)
connect = d.get('time_connect', 0)
appconnect = d.get('time_appconnect', 0)
total = d.get('time_total', 0)
tcp = max(0.0, connect - lookup) * 1000
tls = max(0.0, appconnect - connect) * 1000 if appconnect > 0 else 0.0
print(f'{tls:.3f} {tcp:.3f} {total * 1000:.3f}')
" "$1"
}

submit_timings() {
  local operation="$1"
  local payload="$2"
  local sample_index="$3"
  local inner="${payload#\{}"
  inner="${inner%\}}"
  local meta=""
  meta="\"experiment_id\":\"${EXPERIMENT_ID}\",\"sample_index\":${sample_index}"
  if [[ -n "$ROUTE_MODE" ]]; then
    meta="${meta},\"route_mode\":\"${ROUTE_MODE}\""
  fi
  curl_extra \
    -H 'Content-Type: application/json' \
    --data "{\"operation\":\"${operation}\",\"name\":\"${EXPERIMENT_ID}-${sample_index}\",${meta},${inner}}" \
    "${BASE_URL}/api/results/timings" >/dev/null
}

record_sample() {
  local tls_hs="$1"
  local tcp_ms="$2"
  local total_ms="$3"
  if [[ "$tls_hs" != "0.000" && "$tls_hs" != "0" && -n "$tls_hs" ]]; then
    TLS_HS_SAMPLES+=("$tls_hs")
  fi
  if [[ "$tcp_ms" != "0.000" && "$tcp_ms" != "0" && -n "$tcp_ms" ]]; then
    TCP_SAMPLES+=("$tcp_ms")
  fi
  if [[ "$total_ms" != "0.000" && "$total_ms" != "0" && -n "$total_ms" ]]; then
    TOTAL_SAMPLES+=("$total_ms")
  fi
}

run_handshake_sample() {
  local sample_index="$1"
  local probe_url="${BASE_URL}/api/bench/probe?experiment_id=${EXPERIMENT_ID}&sample_index=${sample_index}"
  if [[ -n "$ROUTE_MODE" ]]; then
    probe_url="${probe_url}&route_mode=${ROUTE_MODE}"
  fi
  local timing
  timing="$(
    curl_extra ${curl_protocol_flags[@]+"${curl_protocol_flags[@]}"} ${curl_conn_flags[@]+"${curl_conn_flags[@]}"} \
      -o "${WORKDIR}/probe-${sample_index}.json" -w "${CURL_JSON}" \
      "${probe_url}"
  )"
  submit_timings handshake "${timing}" "${sample_index}"
  read -r tls_hs tcp_ms total_ms <<< "$(curl_phases_ms "${timing}")"
  record_sample "${tls_hs}" "${tcp_ms}" "${total_ms}"
  echo "sample ${sample_index}: curl ${timing}"
}

run_blob_upload_sample() {
  local sample_index="$1"
  local file="$2"
  local name="$3"
  local timing
  timing="$(
    curl_extra ${curl_protocol_flags[@]+"${curl_protocol_flags[@]}"} ${curl_conn_flags[@]+"${curl_conn_flags[@]}"} \
      ${bench_headers[@]+"${bench_headers[@]}"} \
      -H "X-Experiment-Id: ${EXPERIMENT_ID}" \
      -H "X-Sample-Index: ${sample_index}" \
      -o "${WORKDIR}/upload-${sample_index}.json" -w "${CURL_JSON}" \
      --upload-file "${file}" \
      "${BASE_URL}/api/blobs/${name}"
  )"
  submit_timings upload "${timing}" "${sample_index}"
  read -r tls_hs tcp_ms total_ms <<< "$(curl_phases_ms "${timing}")"
  record_sample "${tls_hs}" "${tcp_ms}" "${total_ms}"
  echo "upload sample ${sample_index}: curl ${timing}"
}

run_blob_download_sample() {
  local sample_index="$1"
  local name="$2"
  local timing
  timing="$(
    curl_extra ${curl_protocol_flags[@]+"${curl_protocol_flags[@]}"} \
      ${bench_headers[@]+"${bench_headers[@]}"} \
      -H "X-Experiment-Id: ${EXPERIMENT_ID}" \
      -H "X-Sample-Index: ${sample_index}" \
      -o "${WORKDIR}/download-${sample_index}.bin" -w "${CURL_JSON}" \
      "${BASE_URL}/api/blobs/${name}"
  )"
  submit_timings download "${timing}" "${sample_index}"
  echo "download sample ${sample_index}: curl ${timing}"
}

print_summary() {
  echo
  echo "== experiment ${EXPERIMENT_ID} summary (after ${WARMUP} warmup discarded) =="
  echo -n "tls_handshake_ms: "
  percentile_summary "${TLS_HS_SAMPLES[@]}"
  echo -n "tcp_connect_ms: "
  percentile_summary "${TCP_SAMPLES[@]}"
  echo -n "client_total_ms: "
  percentile_summary "${TOTAL_SAMPLES[@]}"
}

TOTAL_ITERS=$((WARMUP + REPEAT))

if [[ "$HANDSHAKE_ONLY" == true ]]; then
  echo "== handshake-only probe (${REPEAT} samples, ${WARMUP} warmup) to ${BASE_URL}"
  for ((i = 1; i <= TOTAL_ITERS; i++)); do
    run_handshake_sample "${i}"
  done
  if [[ "$WARMUP" -gt 0 ]]; then
    TLS_HS_SAMPLES=("${TLS_HS_SAMPLES[@]:$WARMUP}")
    TCP_SAMPLES=("${TCP_SAMPLES[@]:$WARMUP}")
    TOTAL_SAMPLES=("${TOTAL_SAMPLES[@]:$WARMUP}")
  fi
  print_summary
  echo
  echo "== info (does the pod see TLS?)"
  curl_extra "${BASE_URL}/api/info"
  echo
  exit 0
fi

NAME="vm-${SIZE}.bin"
FILE="${WORKDIR}/${NAME}"
head -c "${SIZE}" /dev/urandom > "${FILE}"

if [[ "$REPEAT" -eq 1 && "$WARMUP" -eq 0 ]]; then
  echo "== upload ${NAME} (${SIZE} bytes) to ${BASE_URL}"
  UPLOAD_TIMING="$(
    curl_extra ${curl_protocol_flags[@]+"${curl_protocol_flags[@]}"} \
      ${bench_headers[@]+"${bench_headers[@]}"} \
      -H "X-Experiment-Id: ${EXPERIMENT_ID}" \
      -H "X-Sample-Index: 1" \
      -o "${WORKDIR}/upload.json" -w "${CURL_JSON}" \
      --upload-file "${FILE}" \
      "${BASE_URL}/api/blobs/${NAME}"
  )"
  cat "${WORKDIR}/upload.json"
  echo
  echo "curl ${UPLOAD_TIMING}"
  submit_timings upload "${UPLOAD_TIMING}" 1

  echo "== download ${NAME} from ${BASE_URL}"
  DOWNLOAD_TIMING="$(
    curl_extra ${curl_protocol_flags[@]+"${curl_protocol_flags[@]}"} \
      ${bench_headers[@]+"${bench_headers[@]}"} \
      -H "X-Experiment-Id: ${EXPERIMENT_ID}" \
      -H "X-Sample-Index: 1" \
      -o "${WORKDIR}/download.bin" -w "${CURL_JSON}" \
      "${BASE_URL}/api/blobs/${NAME}"
  )"
  echo "curl ${DOWNLOAD_TIMING}"
  submit_timings download "${DOWNLOAD_TIMING}" 1
else
  echo "== upload ${NAME} (${SIZE} bytes) x${REPEAT} (+${WARMUP} warmup) to ${BASE_URL}"
  for ((i = 1; i <= TOTAL_ITERS; i++)); do
    run_blob_upload_sample "${i}" "${FILE}" "${NAME}"
  done
  if [[ "$WARMUP" -gt 0 ]]; then
    TLS_HS_SAMPLES=("${TLS_HS_SAMPLES[@]:$WARMUP}")
    TCP_SAMPLES=("${TCP_SAMPLES[@]:$WARMUP}")
    TOTAL_SAMPLES=("${TOTAL_SAMPLES[@]:$WARMUP}")
  fi
  print_summary

  echo "== download ${NAME} from ${BASE_URL}"
  run_blob_download_sample 1 "${NAME}"
fi

echo "== info (does the pod see TLS?)"
curl_extra "${BASE_URL}/api/info"
echo
