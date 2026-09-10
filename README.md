# OCP4 TLS Routes Benchmarks

A Go HTTPS/HTTP target for OpenShift. Deploy it in-cluster, hit it through a **Service** or a **Route**, and measure TLS encrypt/decrypt plus disk write on a PVC. A small Web UI shows a benchmark table. The image also includes statically linked BusyBox `ping`, `traceroute`, and `nc`, plus `curl`, for `oc exec` debugging (`ping` needs `NET_RAW`, which this image does not grant).

## Documentation

| Guide | Description |
| --- | --- |
| [docs/README.md](docs/README.md) | Index and quick comparison |
| [Running locally](docs/running-locally.md) | Go on your laptop (`make run`) |
| [Running with Podman/Docker](docs/running-with-podman-docker.md) | Container pre-flight before OpenShift |
| [Running on OpenShift](docs/running-on-openshift.md) | PVC, Routes, Secrets |
| [TLS certificates for benchmarks](docs/tls-certificates.md) | RSA 2048/4096, ECDSA; volumes vs Secrets |
| [TLS benchmark methodology](docs/tls-benchmark-methodology.md) | Handshake vs bulk matrices, percentiles, Route modes |

## What you can measure

| Path | Client | What the pod sees |
| --- | --- | --- |
| Service HTTP `:8080` | Another pod | Plain HTTP |
| Service HTTPS `:8443` | Another pod | TLS at the app |
| Route **edge** | Linux VM `curl` | HTTP at the app (TLS at the router) |
| Route **passthrough** | Linux VM `curl` | TLS at the app |
| Route **reencrypt** | Linux VM `curl` | TLS at the app (and TLS at the router) |

**Generate** writes `/dev/urandom` to the PVC (disk baseline). **Upload** (`PUT`) sends bytes from a VM or pod onto the PVC (TLS decrypt + write). **Download** (`GET`) reads the PVC and sends bytes out (TLS encrypt).

## Quick start

```bash
git clone https://github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks.git
cd OCP4-TLS-Routes-Benchmarks
cp .env.example .env   # optional

make test              # Go locally, or Podman golang image
make run               # Go binary, or podman compose up --build
```

Dashboard: <http://127.0.0.1:8080/> — benchmark table with cipher, cert key, client/server TLS handshake, Route mode, ALPN, collapsible experiment groups, and a configurable column picker.

For full benchmark steps, certificates, and OpenShift deployment, use the [docs](docs/README.md).

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | HTTP listener (edge Route, in-cluster HTTP) |
| `HTTPS_ADDR` | `:8443` | TLS listener (passthrough, reencrypt, Service HTTPS) |
| `TLS_CERT_FILE` | `/certs/tls.crt` | Server certificate (PEM) |
| `TLS_KEY_FILE` | `/certs/tls.key` | Server private key (PEM) |
| `TLS_CA_FILE` | `/certs/ca.crt` | CA that signed the server cert; also `GET /ca.crt` |
| `TLS_CA_DIR` | `/etc/pki/internal-ca` | Extra PEM CAs for pod-to-pod trust |
| `TLS_CA_BUNDLE_FILE` | `/certs/ca-bundle.pem` | Bundle built at container start |
| `TLS_CLIENT_CA_FILE` | empty | If set, HTTPS requires a client certificate |
| `TLS_DNS_NAMES` | `localhost` | SANs when generating a lab cert (add the Route host) |
| `DATA_DIR` | `/data` | PVC mount (blobs + results) |
| `MAX_BLOB_BYTES` | `268435456` | Max generate/upload size (256 MiB) |
| `RESULTS_LOG` | `/data/results/runs.jsonl` | Append-only run log for the UI |

If `TLS_CERT_FILE` / `TLS_KEY_FILE` are missing, the process generates an internal **ECDSA P-256** CA and server certificate. For RSA or other profiles, use [`scripts/gen-certs.sh`](scripts/gen-certs.sh) — see [docs/tls-certificates.md](docs/tls-certificates.md).

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/` | Dashboard |
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness |
| `GET` | `/ca.crt` | Trust anchor for VM `--cacert` |
| `GET` | `/api/info` | TLS vs HTTP as seen by the pod |
| `GET` | `/api/results` | JSONL-backed benchmark rows |
| `GET` | `/api/bench/probe?experiment_id=&sample_index=&route_mode=` | Record handshake sample + return connection info |
| `POST` | `/api/results/timings` | Attach curl phases (`experiment_id`, `sample_index`, `route_mode`) |
| `GET` | `/api/blobs` | List PVC files |
| `POST` | `/api/blobs?name=&size=` | Generate urandom onto the PVC |
| `PUT` | `/api/blobs/{name}` | Upload (curl `--upload-file`) |
| `GET` | `/api/blobs/{name}` | Download |
| `DELETE` | `/api/blobs/{name}` | Delete |

## Makefile

`make help`, `build`, `test`, `run`, `image`, `compose-up`, `compose-down`, `fmt`, `vet`. Without a local Go toolchain, `make test` and `make run` use Podman or Docker.
