# OCP4 TLS Routes Benchmarks

A Go HTTPS/HTTP target for OpenShift. Deploy it in-cluster, hit it through a **Service** or a **Route**, and measure TLS encrypt/decrypt plus disk write on a PVC. A small Web UI shows a benchmark table. The image also includes `ping`, `traceroute`, `nc`, and `curl` for `oc exec` debugging.

## What you can measure

| Path | Client | What the pod sees |
| --- | --- | --- |
| Service HTTP `:8080` | Another pod | Plain HTTP |
| Service HTTPS `:8443` | Another pod | TLS at the app |
| Route **edge** | Linux VM `curl` | HTTP at the app (TLS at the router) |
| Route **passthrough** | Linux VM `curl` | TLS at the app |
| Route **reencrypt** | Linux VM `curl` | TLS at the app (and TLS at the router) |

**Generate** writes `/dev/urandom` to the PVC (disk baseline). **Upload** (`PUT`) sends bytes from a VM or pod onto the PVC (TLS decrypt + write). **Download** (`GET`) reads the PVC and sends bytes out (TLS encrypt).

## Install

### Local

Go 1.23+ is optional. If `go` is not on your `PATH`, `make test` and `make run` use Podman or Docker instead.

```bash
git clone https://github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks.git
cd OCP4-TLS-Routes-Benchmarks
cp .env.example .env   # optional
make test
make run
```

Open [http://127.0.0.1:8080/](http://127.0.0.1:8080/) for the dashboard. HTTPS is on [https://127.0.0.1:8443/](https://127.0.0.1:8443/) (self-signed).

To install Go later: https://go.dev/dl/ or `brew install go`.

### Container

```bash
make image                 # IMAGE=tlsbench:dev (podman or docker)
make compose-up            # publishes 8080 and 8443
```

### OpenShift

1. Build and push the image to a registry your cluster can pull, then set `spec.template.spec.containers[0].image` in `deploy/openshift/03-deployment.yaml`.
2. Apply manifests:

```bash
oc apply -k deploy/openshift
```

3. Patch the reencrypt Route CA after the pod has written `/certs/ca.crt`:

```bash
./scripts/patch-reencrypt-ca.sh tlsbench tlsbench tlsbench-reencrypt
```

4. Optional: mount your own server cert (`deploy/openshift/secret-tls.example.yaml`) and extra internal CAs on ConfigMap `tlsbench-internal-ca`.

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

If `TLS_CERT_FILE` / `TLS_KEY_FILE` are missing, the process generates an internal CA and server certificate and writes them to those paths.

`TLS_CA_DIR` is concatenated into `TLS_CA_BUNDLE_FILE`. The image sets `SSL_CERT_FILE` and `CURL_CA_BUNDLE` so `oc exec -- curl https://other-svc.ns.svc` can trust cluster-internal CAs.

## Use the Web UI

Open `/` on HTTP or HTTPS. The table lists generate, upload, and download runs from the PVC log (time, size, TLS at the pod, client address, write/read/total ms, MiB/s). Filters and auto-refresh are in the page. The UI does not run ping or curl; it only displays stored results.

`GET /api/info` shows whether **this** request arrived with TLS at the pod (passthrough / reencrypt / Service HTTPS) or as HTTP with `X-Forwarded-*` (edge).

## Benchmark examples

Sizes: 1 KiB (`1024`), 1 MiB (`1048576`), 10 MiB (`10485760`). Repeat on edge vs passthrough and compare the UI table (or `GET /api/results`).

### Disk-only generate (inside the cluster)

```bash
oc exec -n tlsbench deploy/tlsbench -- \
  curl -sS -X POST 'http://127.0.0.1:8080/api/blobs?name=1mb.bin&size=1048576'
```

### Linux VM upload/download over a Route

Save the app CA for passthrough (optional):

```bash
curl -k https://tlsbench-passthrough.apps.example.com/ca.crt -o ca.crt
```

```bash
# Edge: TLS to the router, HTTP to the pod
./scripts/vm-bench.sh https://tlsbench-edge.apps.example.com 1048576

# Passthrough: TLS to the app
./scripts/vm-bench.sh https://tlsbench-passthrough.apps.example.com 1048576 --cacert ca.crt
# or, for a lab only:
./scripts/vm-bench.sh https://tlsbench-passthrough.apps.example.com 1048576 -k
```

Manual curl:

```bash
head -c 1048576 /dev/urandom > 1mb.bin
curl -sS --upload-file 1mb.bin \
  "https://tlsbench-edge.apps.example.com/api/blobs/1mb.bin"
curl -sS -o /tmp/1mb.bin \
  "https://tlsbench-edge.apps.example.com/api/blobs/1mb.bin"
```

### In-cluster Service (pod-to-pod)

```bash
oc exec -n tlsbench deploy/tlsbench -- \
  sh -c 'head -c 1048576 /dev/urandom | curl -sS --upload-file - \
    http://tlsbench.tlsbench.svc:8080/api/blobs/svc-http.bin'

oc exec -n tlsbench deploy/tlsbench -- \
  sh -c 'head -c 1048576 /dev/urandom | curl -sS --cacert /certs/ca.crt --upload-file - \
    https://tlsbench.tlsbench.svc:8443/api/blobs/svc-https.bin'
```

Refresh the UI and compare **upload HTTP vs HTTPS** at the same size. The TLS column is whether the **pod** decrypted TLS, not whether the VM used `https://`.

### Network tools in the image

```bash
oc exec -n tlsbench -it deploy/tlsbench -- ping -c 3 dns.default.svc
oc exec -n tlsbench -it deploy/tlsbench -- traceroute -n 10.128.0.1
oc exec -n tlsbench -it deploy/tlsbench -- nc -vz tlsbench.tlsbench.svc 8443
oc exec -n tlsbench -it deploy/tlsbench -- curl -svk https://tlsbench.tlsbench.svc:8443/api/info
```

Default OpenShift `restricted-v2` SCC drops `NET_RAW`, so `ping` / `traceroute` may fail while `curl` and `nc` still work.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/` | Dashboard |
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness |
| `GET` | `/ca.crt` | Trust anchor for VM `--cacert` |
| `GET` | `/api/info` | TLS vs HTTP as seen by the pod |
| `GET` | `/api/results` | JSONL-backed benchmark rows |
| `GET` | `/api/blobs` | List PVC files |
| `POST` | `/api/blobs?name=&size=` | Generate urandom onto the PVC |
| `PUT` | `/api/blobs/{name}` | Upload (curl `--upload-file`) |
| `GET` | `/api/blobs/{name}` | Download |
| `DELETE` | `/api/blobs/{name}` | Delete |

## Makefile

`make help`, `build`, `test`, `run`, `image`, `compose-up`, `compose-down`, `fmt`, `vet`. Override `IMAGE` and `GOFLAGS` as needed. Without a local Go toolchain, `make test` and `make run` use Podman or Docker.
