# Running TLS benchmarks locally (Go)

Use this path when Go 1.23+ is installed. Handshake matrices, `--reuse`, and flag meanings live in [TLS benchmark methodology](tls-benchmark-methodology.md). Cert profiles live in [TLS certificates](tls-certificates.md).

## Prerequisites

- Go 1.23+ — [go.dev/dl](https://go.dev/dl/) or `brew install go`
- `curl`

## Setup and run

```bash
git clone https://github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks.git
cd OCP4-TLS-Routes-Benchmarks
cp .env.example .env    # optional; ./data and ./certs
make test
make run
```

- HTTP: <http://127.0.0.1:8080/>
- HTTPS: <https://127.0.0.1:8443/> (self-signed ECDSA P-256 if `./certs/tls.crt` is missing)

Open the [dashboard](http://127.0.0.1:8080/). Repeated `experiment_id` samples collapse; **Columns** and the metrics guide are in the UI.

```bash
curl -sS http://127.0.0.1:8080/api/info | jq .          # edge-like: tls=false
curl -sk https://127.0.0.1:8443/api/info | jq .           # passthrough-like: tls=true
```

## Blob smoke tests

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/blobs?name=1mb.bin&size=1048576'
head -c 1048576 /dev/urandom > /tmp/1mb.bin
curl -sS --upload-file /tmp/1mb.bin http://127.0.0.1:8080/api/blobs/upload-http.bin
curl -sk --upload-file /tmp/1mb.bin https://127.0.0.1:8443/api/blobs/upload-https.bin
curl -sS -o /tmp/out-http.bin http://127.0.0.1:8080/api/blobs/upload-https.bin
curl -sk -o /tmp/out-https.bin https://127.0.0.1:8443/api/blobs/upload-https.bin
curl -sS http://127.0.0.1:8080/api/results | jq .
```

## Timed runs

Raw curl does not fill TLS hs cli. Use the wrapper (sizes: `1024`, `1048576`, `10485760`):

```bash
./scripts/vm-bench.sh http://127.0.0.1:8080 1048576
./scripts/vm-bench.sh --handshake-only --http1.1 --route-mode service-https \
  --repeat 30 --warmup 3 https://127.0.0.1:8443 0 -k
```

Handshake vs bulk, `--http2` / `--reuse`, and why reuse TLS hs is not a cold p50: [methodology](tls-benchmark-methodology.md).

To pin a cert profile before `make run`:

```bash
./scripts/gen-certs.sh ecdsa-p256 ./certs/ecdsa-p256 localhost
export TLS_CERT_FILE=./certs/ecdsa-p256/tls.crt TLS_KEY_FILE=./certs/ecdsa-p256/tls.key TLS_CA_FILE=./certs/ecdsa-p256/ca.crt
```

Stop with `Ctrl+C`. Next: [Podman/Docker](running-with-podman-docker.md), then [OpenShift](running-on-openshift.md).
