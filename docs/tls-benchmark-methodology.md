# TLS benchmark methodology

Use this protocol to compare **cert key algorithms** (RSA vs ECDSA) and **Route termination** (edge, passthrough, reencrypt) without mixing handshake cost with PVC disk I/O.

## What each metric means

| Metric | Source | Measures |
| --- | --- | --- |
| **TLS hs cli** | curl `time_appconnect − time_connect` posted to `/api/results/timings` | Client-side handshake (router on edge/reencrypt). Empty until timings are merged; never copied from the pod Accept clock. |
| **TLS hs srv** | Pod Accept handshake | Server-side handshake (pod on passthrough/Service HTTPS; router↔pod on reencrypt) |
| **Xfer ms** | Upload: curl pretransfer → starttransfer (send + server wait). Download: curl body transfer | Bulk path duration; not cert signature cost |
| **MiB/s** | bytes ÷ bulk interval (upload: TTFB; download: transfer) | Throughput excluding DNS/TCP/handshake |

The UI **TLS** column means TLS **at the pod**, not at the client URL alone. Edge Routes show HTTP at the pod even though the client used HTTPS to the router.

**Cipher** is the Go IANA name (for example `TLS_AES_128_GCM_SHA256` or `TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256`), not the OpenSSL TLS 1.2 name (`ECDHE-ECDSA-AES128-GCM-SHA256`). Compare like-for-like names when mixing `openssl s_client` with the dashboard.

## Dashboard

- **Metrics guide** — expandable panel with good/caution/concern ranges per metric
- **Collapse repeats** — groups samples by `experiment_id` into one p50/p99 summary row; click ▶ to expand
- **Columns** — show/hide table columns (saved in browser localStorage)
- **Filters** — operation, TLS at pod, Route mode
- Color hints on TLS hs, TCP, and MiB/s are rough guides for same-cluster runs

## Handshake matrix (cert key comparison)

Isolate signature cost with handshake-only probes. **`--http1.1` is required** so ALPN does not switch to HTTP/2 mid-study (`vm-bench.sh` exits without it unless you pass `--http2` or `--reuse`).

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

Default: **cold** handshake per sample — a new curl process with `--no-keepalive` and `--no-sessionid` (full TCP+TLS each time). The server disables TLS session tickets by default (`TLS_DISABLE_SESSION_TICKETS=true`) so cold tables are not contaminated by resumption.

`--reuse` is HTTP/2 multiplex, not keep-alive across separate curl processes. It requires `--handshake-only` and is incompatible with `--http1.1`. One curl process opens a single connection and issues `--repeat` probe requests as concurrent streams (`curl --http2 --parallel`). Warmup is unused because there is only one handshake.

```bash
./scripts/vm-bench.sh --handshake-only --reuse --repeat 30 \
  --route-mode service-https https://127.0.0.1:8443 0 -k
```

Expect **one** sample with TCP/TLS handshake cost; the rest should show **Reused**, empty **TLS hs cli**, and `num_connects=0`. HTTP/1.1 cannot multiplex streams on one connection — keep `--http1.1` for cert-key tables.

### Why TLS hs looks higher with `--reuse`

Do not compare `--reuse` **TLS hs cli** to the cold-handshake p50. They are different measurements.

Cold `--handshake-only --http1.1 --repeat 30 --warmup 3` discards the first samples, then reports percentiles across many full handshakes (often ~3–4 ms on localhost). `--reuse` has **one** handshake (`count: 1` in the script summary) and **no warmup**, so that single connection setup is the entire TLS number.

That setup sample is also slower as a **timing artifact**, not heavier crypto:

- curl `--parallel` **arms every stream at once**, then completes TLS. `time_appconnect − time_connect` includes wait until the ClientHello finishes while N streams are queued. The pod's **TLS hs srv** matches because `HandshakeContext` waits on the same ClientHello.
- Follow-up streams do **not** pay that cost: **TLS hs cli/srv** empty, **Reused**, `num_connects=0`.
- **Xfer ms** / **client total** on every multiplex sample can look high because all streams share that start time. That is queueing on one connection, not N handshakes.
- The same host with **serial** URLs in one curl process (no `--parallel`) typically shows a first-stream TLS hs close to the cold p50, then reused streams.

Use `--reuse` to confirm multiplex. Use cold `--http1.1` plus warmup for cert-key and cipher handshake tables.

## OpenShift HTTP/2 notes

- Client HTTP/2 to the router usually needs a **custom unique Route certificate**; default ingress certs may force HTTP/1.1.
- End-to-end HTTP/2 through HAProxy depends on Route type; see [Running on OpenShift](running-on-openshift.md).
- Pin `--http1.1` in `vm-bench.sh` when comparing cipher or cert results.
- Optional server pin: `TLS_CIPHER_SUITES=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256` (Go IANA names). TLS 1.3: `TLS_AES_128_GCM_SHA256`. Apple curl may ignore `--tls13-ciphers`; prefer server-side pin or `--tls-max 1.2`.

## Rules for valid tables

1. Change **one variable** per table (Route mode, cert profile, payload size, or ALPN).
2. Discard warmup (`--warmup 3` when `--repeat > 1`). `--reuse` has no warmup; do not compare its single TLS hs to a cold p50.
3. Do not compare a 100 MiB PVC upload to a handshake probe.
4. Label every run with `--route-mode`.
5. On reencrypt, compare **both** client and server handshake columns.

Probe and timings APIs: root [README](../README.md#api). Cert files: [TLS certificates](tls-certificates.md). Deploy: [Running on OpenShift](running-on-openshift.md).
