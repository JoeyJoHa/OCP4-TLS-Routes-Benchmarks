# TLS benchmark guides

Step-by-step guides for running **OCP4 TLS Routes Benchmarks** in three environments.

| Guide | Use when |
| --- | --- |
| [Running locally](running-locally.md) | You have Go installed and want the fastest dev loop on your laptop |
| [Running with Podman/Docker](running-with-podman-docker.md) | You want to test the **same container image** before OpenShift (recommended pre-flight) |
| [Running on OpenShift](running-on-openshift.md) | You deploy to a cluster with PVC, Service, Routes, and **TLS Secrets** |
| [TLS certificates for benchmarks](tls-certificates.md) | You want RSA 2048/4096, ECDSA, or custom certs (volume locally, Secret on OpenShift) |
| [TLS benchmark methodology](tls-benchmark-methodology.md) | Handshake vs bulk matrices, percentiles, and Route mode labeling |

## What you are measuring

- **Generate** — `/dev/urandom` written to disk (PVC baseline, no network)
- **Upload** (`PUT`) — client sends bytes; pod decrypts TLS (if HTTPS) and writes to PVC
- **Download** (`GET`) — pod reads PVC and sends bytes; TLS encrypt on HTTPS

Results appear in the Web UI (`/`) and in `GET /api/results` (JSONL on the PVC).

The dashboard includes a **metrics guide** (good/caution/concern ranges), **collapsible experiment rows** (repeated `experiment_id` samples collapse to p50/p99), filters by operation/TLS/Route mode, and a **Columns** picker (saved in your browser).

## Quick comparison

| | Local (Go) | Podman/Docker | OpenShift |
| --- | --- | --- | --- |
| Image | binary in `bin/` | `tlsbench:dev` | pushed image + manifests |
| Storage | `./data` | compose volume | PVC |
| TLS certs | auto or `./certs` | volume mount | Secret or auto-generated |
| Routes | N/A | N/A | edge / passthrough / reencrypt |
| External client | `curl localhost` | `curl localhost` | Linux VM → Route |

Start with **Podman** if you do not have Go installed. Move to **OpenShift** once upload/download and the dashboard look correct locally.
