# TLS benchmark methodology

Use this protocol to compare **cert key algorithms** (RSA vs ECDSA) and **Route termination** (edge, passthrough, reencrypt) without mixing handshake cost with PVC disk I/O.

## What each metric means

| Metric | Source | Measures |
| --- | --- | --- |
| **TLS hs cli** | curl `time_appconnect − time_connect` | Client-side handshake (router on edge/reencrypt) |
| **TLS hs srv** | Pod Accept handshake | Server-side handshake (pod on passthrough/Service HTTPS; router↔pod on reencrypt) |
| **Xfer ms** | Upload: curl pretransfer → starttransfer (send + server wait). Download: curl body transfer | Bulk path duration; not cert signature cost |
| **MiB/s** | bytes ÷ bulk interval (upload: TTFB; download: transfer) | Throughput excluding DNS/TCP/handshake |

The UI **TLS** column means TLS **at the pod**, not at the client URL alone. Edge Routes show HTTP at the pod even though the client used HTTPS to the router.

## Dashboard

- **Metrics guide** — expandable panel with good/caution/concern ranges per metric
- **Collapse repeats** — groups samples by `experiment_id` into one p50/p99 summary row; click ▶ to expand
- **Columns** — show/hide table columns (saved in browser localStorage)
- **Filters** — operation, TLS at pod, Route mode
- Color hints on TLS hs, TCP, and MiB/s are rough guides for same-cluster runs

## Handshake matrix (cert key comparison)

Isolate signature cost with handshake-only probes. Pin HTTP/1.1 so ALPN does not switch to HTTP/2 mid-study.

```bash
# Passthrough or Service HTTPS — client hs ≈ server hs
./scripts/vm-bench.sh --handshake-only --http1.1 --repeat 30 --warmup 3 \
  --route-mode passthrough https://tlsbench-passthrough.apps.example.com 0 --cacert ca.crt

# Edge — client hs = router terminate; pod sees HTTP (no server hs)
./scripts/vm-bench.sh --handshake-only --http1.1 --repeat 30 --warmup 3 \
  --route-mode edge https://tlsbench-edge.apps.example.com 0

# Reencrypt — client hs ≠ server hs (outer vs inner)
./scripts/vm-bench.sh --handshake-only --http1.1 --repeat 30 --warmup 3 \
  --route-mode reencrypt https://tlsbench-reencrypt.apps.example.com 0
```

Swap only the mounted cert profile (`ecdsa-p256`, `rsa-2048`, `rsa-4096`) and compare **TLS hs cli** and **TLS hs srv** p50/p99 in the dashboard (group experiments enabled).

The script prints a percentile summary and stores each sample under a shared `experiment_id`.

## Bulk matrix (cipher comparison)

After handshake baselines, measure transfer at fixed sizes (1 MiB and 10 MiB):

```bash
./scripts/vm-bench.sh --http1.1 --repeat 10 --warmup 3 --route-mode passthrough \
  https://127.0.0.1:8443 1048576 -k
```

Compare **Xfer ms** and **MiB/s**. At large sizes this reflects negotiated cipher bulk crypto, not cert key type.

## Cold vs reused connections

Default: **cold** handshake per sample (`curl --no-keepalive`). To test session reuse on the same TCP/TLS connection:

```bash
./scripts/vm-bench.sh --handshake-only --reuse --repeat 30 \
  --route-mode service-https https://127.0.0.1:8443 0 -k
```

Rows may show **Reused** and empty client handshake ms on follow-up requests.

## Companion tools (optional)

These are not integrated into the dashboard but useful for cross-checks:

```bash
# Handshake rate only (no HTTP body) — OpenSSL
openssl s_time -connect tlsbench-passthrough.apps.example.com:443 -time 30 -new

# Percentiles with HTTP/1.1 pinned — nghttp2 h2load
h2load --h1 -n 200 -c 1 https://tlsbench-passthrough.apps.example.com/api/info
```

## OpenShift HTTP/2 notes

- Client HTTP/2 to the router usually requires a **custom unique Route certificate**; default ingress certs may force HTTP/1.1.
- End-to-end HTTP/2 through HAProxy to the pod is limited by Route type; see [Running on OpenShift](running-on-openshift.md).
- Use `--http1.1` in `vm-bench.sh` when comparing cipher or cert results across runs.

## Rules for valid tables

1. Change **one variable** per table (Route mode, cert profile, payload size, or ALPN).
2. Discard warmup samples (`--warmup 3` default when `--repeat > 1`).
3. Do not compare a 100 MiB PVC upload (disk-bound) to a handshake probe.
4. Label every run with `--route-mode` so filters work in the UI.
5. On reencrypt, always compare **both** client and server handshake columns.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/bench/probe?experiment_id=&sample_index=&route_mode=` | Record handshake run + return `/api/info` JSON |
| `POST` | `/api/results/timings` | Attach curl phases; include `experiment_id`, `sample_index`, `route_mode` |

See also [TLS certificates for benchmarks](tls-certificates.md) and [Running on OpenShift](running-on-openshift.md).
