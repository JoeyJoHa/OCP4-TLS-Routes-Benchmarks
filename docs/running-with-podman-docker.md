# Running TLS benchmarks with Podman or Docker

Use this guide to test the **same container image** you will deploy on OpenShift, without a local Go install.

## Prerequisites

- Podman or Docker
- `curl` on your workstation
- Optional: `podman compose` or `docker compose`

The Makefile picks Podman when both are installed.

## Quick start (recommended)

```bash
git clone https://github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks.git
cd OCP4-TLS-Routes-Benchmarks
cp .env.example .env    # optional

make test               # unit tests in a golang container if go is missing
make run                # podman compose up --build
```

Open the [dashboard](http://127.0.0.1:8080/) over HTTP. Use [HTTPS](https://127.0.0.1:8443/) with `-k` for self-signed certs.

Stop:

```bash
make compose-down
```

## What Podman tests vs OpenShift

| Checked locally | OpenShift equivalent |
| --- | --- |
| Container starts | Deployment + probes |
| Port 8080 / 8443 | Service `http` / `https` |
| Volume `tlsbench-data` → `/data` | PVC `tlsbench-data` |
| Volume `tlsbench-certs` → `/certs` | emptyDir or TLS Secret |
| `curl` upload/download | VM or pod client |
| `/api/info` TLS flag | passthrough vs edge behavior |

Podman does **not** emulate OpenShift Routes (edge / passthrough / reencrypt). Test those on the cluster.

## Build the image only

```bash
make image                # IMAGE=tlsbench:dev
podman images | grep tlsbench
```

## Run without compose (manual)

```bash
podman run --rm -d --name tlsbench \
  -p 8080:8080 -p 8443:8443 \
  -v tlsbench-data:/data \
  -v tlsbench-certs:/certs \
  -e TLS_DNS_NAMES=localhost \
  tlsbench:dev

podman logs -f tlsbench
podman stop tlsbench
```

## Verify health and TLS paths

```bash
curl -sS http://127.0.0.1:8080/healthz
curl -sS http://127.0.0.1:8080/api/info | jq .
curl -sk https://127.0.0.1:8443/api/info | jq .
```

## Benchmark workflow

### Generate on disk (PVC baseline)

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/blobs?name=1mb.bin&size=1048576'
```

### Upload from your laptop (simulates external VM client)

```bash
head -c 1048576 /dev/urandom > /tmp/1mb.bin

# HTTP — no TLS at the pod
curl -sS --upload-file /tmp/1mb.bin \
  http://127.0.0.1:8080/api/blobs/vm-upload-http.bin

# HTTPS — TLS decrypt at the pod
curl -sk --upload-file /tmp/1mb.bin \
  https://127.0.0.1:8443/api/blobs/vm-upload-https.bin
```

### Download

```bash
curl -sS -o /tmp/dl-http.bin \
  http://127.0.0.1:8080/api/blobs/vm-upload-https.bin

curl -sk -o /tmp/dl-https.bin \
  https://127.0.0.1:8443/api/blobs/vm-upload-https.bin
```

### Script wrapper

Same flow as a future OpenShift Route test (point URL at localhost for Podman):

```bash
./scripts/vm-bench.sh http://127.0.0.1:8080 1048576
./scripts/vm-bench.sh https://127.0.0.1:8443 1048576 -k
```

Refresh the [dashboard](http://127.0.0.1:8080/) and compare rows (cipher, key, DNS, TCP, TLS handshake, TTFB, transfer, MiB/s).

## Exec into the container (like `oc exec`)

```bash
podman exec -it tlsbench sh

# inside the container:
curl -sS http://127.0.0.1:8080/healthz
curl -sk https://127.0.0.1:8443/api/info
ping -c 2 127.0.0.1
nc -vz 127.0.0.1 8443
```

## TLS certificates with different algorithms (Podman)

For algorithm benchmarks (RSA 2048, RSA 4096, ECDSA), generate a cert set and mount it instead of the auto-generated volume.

```bash
./scripts/gen-certs.sh rsa-2048 ./certs-rsa2048 localhost
```

Run with that cert directory:

```bash
podman run --rm -d --name tlsbench-rsa \
  -p 8080:8080 -p 8443:8443 \
  -v tlsbench-data:/data \
  -v "$(pwd)/certs-rsa2048:/certs:ro" \
  -e TLS_CERT_FILE=/certs/tls.crt \
  -e TLS_KEY_FILE=/certs/tls.key \
  -e TLS_CA_FILE=/certs/ca.crt \
  -e TLS_DNS_NAMES=localhost \
  tlsbench:dev
```

Re-run the same upload/download sizes; only the server key type changes. Details: [TLS certificates for benchmarks](tls-certificates.md).

## Troubleshooting

| Issue | Fix |
| --- | --- |
| `go: command not found` on `make test` / `make run` | Expected — Makefile uses Podman automatically |
| Slow first build | Pulls `golang:1.23-bookworm` and `busybox`; later builds use cache |
| HTTPS curl fails | Use `-k` or `curl --cacert` with `curl -sk https://127.0.0.1:8443/ca.crt -o ca.crt` |
| Port already in use | `podman compose down` or change ports in `docker-compose.yml` |

## Pre-OpenShift checklist

1. `make test` passes  
2. `make run` — dashboard loads  
3. Upload + download over HTTP and HTTPS work  
4. Results appear in the UI table  
5. `make image` — tag and push to a registry the cluster can pull  

Then follow [Running on OpenShift](running-on-openshift.md).
