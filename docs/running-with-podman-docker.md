# Running TLS benchmarks with Podman or Docker

Same **container image** as OpenShift, without a local Go install. Localhost curl recipes: [Running locally](running-locally.md). Handshake tables: [methodology](tls-benchmark-methodology.md). Cert mounts: [TLS certificates](tls-certificates.md).

## Prerequisites

Podman or Docker, plus `curl`. The Makefile prefers Podman when both are installed.

## Quick start

```bash
git clone https://github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks.git
cd OCP4-TLS-Routes-Benchmarks
cp .env.example .env    # optional
make test               # golang container if `go` is missing
make run                # compose up --build
```

Dashboard: <http://127.0.0.1:8080/>. HTTPS: <https://127.0.0.1:8443/> with `-k`. Stop: `make compose-down`.

| Checked here | OpenShift equivalent |
| --- | --- |
| Container starts | Deployment + probes |
| Ports 8080 / 8443 | Service `http` / `https` |
| Volume → `/data` | PVC |
| Volume → `/certs` | emptyDir or TLS Secret |

Podman does **not** emulate Routes. Test edge / passthrough / reencrypt on the cluster.

## Image and manual run

```bash
make image
podman run --rm -d --name tlsbench \
  -p 8080:8080 -p 8443:8443 \
  -v tlsbench-data:/data \
  -v tlsbench-certs:/certs \
  -e TLS_DNS_NAMES=localhost \
  tlsbench:dev
podman logs -f tlsbench
podman stop tlsbench
```

Health: `curl -sS http://127.0.0.1:8080/healthz`. Timed client: `./scripts/vm-bench.sh https://127.0.0.1:8443 1048576 -k`.

```bash
podman exec -it tlsbench sh
# curl / nc inside; ping needs NET_RAW (not granted)
```

`nc` comes from `busybox:1.36-uclibc`. `GLIBC_2.38 not found` means an old glibc BusyBox image — rebuild.

## Troubleshooting

| Issue | Fix |
| --- | --- |
| `go: command not found` | Makefile uses Podman for `test` / `run` |
| Slow first build | Pulls `golang:1.23-bookworm` and `busybox:1.36-uclibc` |
| `ping: permission denied` | Expected. Use `nc -vz` or `curl` |
| HTTPS curl fails | `-k` or `--cacert` after `curl -sk https://127.0.0.1:8443/ca.crt -o ca.crt` |
| Port in use | `make compose-down` or change `docker-compose.yml` |

Pre-OpenShift: tests pass, dashboard loads, HTTP and HTTPS upload/download appear in the UI, image is tagged for the cluster registry. Then [Running on OpenShift](running-on-openshift.md).
