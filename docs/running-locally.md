# Running TLS benchmarks locally (Go)

Use this path when Go 1.23+ is installed and you want the quickest edit/test loop without building a container.

## Prerequisites

- Go 1.23+ — https://go.dev/dl/ or `brew install go`
- `curl` on your workstation

## Setup

```bash
git clone https://github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks.git
cd OCP4-TLS-Routes-Benchmarks
cp .env.example .env    # optional; sets ./data and ./certs paths
```

## Build and run

```bash
make test
make run
```

The app listens on:

- HTTP: http://127.0.0.1:8080/
- HTTPS: https://127.0.0.1:8443/ (self-signed ECDSA P-256 if no cert files exist)

On first start, if `./certs/tls.crt` and `./certs/tls.key` are missing, the app generates a lab CA and server certificate under `./certs/`.

## Open the dashboard

http://127.0.0.1:8080/

The benchmark table fills after you run generate, upload, or download tests below. Repeated runs with the same `experiment_id` collapse to one summary row (click ▶ to expand samples). Use **Columns** to hide noisy fields; the **metrics guide** explains TLS hs cli/srv, MiB/s, and color hints.

## Verify HTTP vs HTTPS at the pod

```bash
# Plain HTTP — same as an edge Route at the pod
curl -sS http://127.0.0.1:8080/api/info | jq .

# TLS at the app — same as passthrough / Service HTTPS
curl -sk https://127.0.0.1:8443/api/info | jq .
```

Check `tls`, `tls_version`, and `cipher` in the JSON.

## Run benchmark scenarios

### 1. Disk-only generate (no client network)

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/blobs?name=1mb.bin&size=1048576'
```

### 2. Upload over HTTP

```bash
head -c 1048576 /dev/urandom > /tmp/1mb.bin
curl -sS --upload-file /tmp/1mb.bin \
  http://127.0.0.1:8080/api/blobs/upload-http.bin
```

### 3. Upload over HTTPS (TLS decrypt + disk write)

```bash
curl -sk --upload-file /tmp/1mb.bin \
  https://127.0.0.1:8443/api/blobs/upload-https.bin
```

### 4. Download over HTTP / HTTPS

```bash
curl -sS -o /tmp/out-http.bin \
  http://127.0.0.1:8080/api/blobs/upload-https.bin

curl -sk -o /tmp/out-https.bin \
  https://127.0.0.1:8443/api/blobs/upload-https.bin
```

### 5. View results

- Web UI: <http://127.0.0.1:8080/>
- API: `curl -sS http://127.0.0.1:8080/api/results | jq .`

For DNS, TCP connect, TLS handshake, TTFB, and transfer phases (needed to compare ciphers and cert keys), use the wrapper instead of raw curl:

```bash
./scripts/vm-bench.sh http://127.0.0.1:8080 1048576
./scripts/vm-bench.sh https://127.0.0.1:8443 1048576 -k

# Handshake-only percentiles (cert key comparison)
./scripts/vm-bench.sh --handshake-only --http1.1 --route-mode service-https \
  --repeat 30 --warmup 3 https://127.0.0.1:8443 0 -k

# Repeated bulk upload with summary (512 KiB × 10)
./scripts/vm-bench.sh --http1.1 --repeat 10 --warmup 3 --route-mode service-https \
  https://127.0.0.1:8443 524288 -k
```

### vm-bench options

| Flag | Purpose |
| --- | --- |
| `--handshake-only` | `GET /api/bench/probe` only (no blob I/O) |
| `--repeat N` | Samples after warmup (default 30 for handshake, 1 for blob) |
| `--warmup N` | Discarded samples (default 3 when `--repeat > 1`) |
| `--http1.1` | Pin ALPN to HTTP/1.1 |
| `--reuse` | Keep-alive / session reuse (default: cold per sample) |
| `--route-mode` | Label rows: `edge`, `passthrough`, `reencrypt`, `service-http`, `service-https` |
| `--experiment-id` | Group samples in the dashboard |

Upload **MiB/s** uses the send interval (TTFB); download **MiB/s** uses the response body transfer interval. See [TLS benchmark methodology](tls-benchmark-methodology.md) for full matrices.

## Payload sizes for tables

Repeat the same steps with different sizes:

| Label | Bytes |
| --- | --- |
| 1 KiB | `1024` |
| 1 MiB | `1048576` |
| 10 MiB | `10485760` |

Build a table: rows = size, columns = upload HTTP, upload HTTPS, download HTTP, download HTTPS.

## Custom certificates (RSA / ECDSA)

By default the app auto-generates **ECDSA P-256**. To benchmark other key types, generate PEM files and point env vars at them before `make run`:

```bash
./scripts/gen-certs.sh ecdsa-p256 ./certs-ecdsa localhost
export TLS_CERT_FILE=./certs-ecdsa/tls.crt
export TLS_KEY_FILE=./certs-ecdsa/tls.key
export TLS_CA_FILE=./certs-ecdsa/ca.crt
make run
```

See [TLS certificates for benchmarks](tls-certificates.md) for RSA 2048, RSA 4096, and OpenShift Secret workflows.

## Stop

`Ctrl+C` in the terminal running `make run`.

## Next step

When local tests pass, move to [Running with Podman/Docker](running-with-podman-docker.md) to validate the container image, then [Running on OpenShift](running-on-openshift.md).
