# Running TLS benchmarks on OpenShift

PVC + Service + Routes, then clients from a Linux VM or another pod. Handshake matrices: [methodology](tls-benchmark-methodology.md). Cert Secrets: [TLS certificates](tls-certificates.md).

## Prerequisites

OpenShift 4.x, `oc` logged in, image in a registry the cluster can pull. Optional: Linux VM with `curl` and Route DNS.

## 1. Build, push, deploy

```bash
make image
podman tag tlsbench:dev quay.io/<user>/tlsbench:latest
podman push quay.io/<user>/tlsbench:latest
```

Set `image:` / `imagePullPolicy: Always` in `deploy/openshift/03-deployment.yaml`, then:

```bash
oc apply -k deploy/openshift
oc wait -n tlsbench --for=condition=ready pod -l app.kubernetes.io/name=tlsbench --timeout=120s
oc get route -n tlsbench
```

| Route | Termination | Pod sees |
| --- | --- | --- |
| `tlsbench-edge` | edge | HTTP |
| `tlsbench-passthrough` | passthrough | TLS |
| `tlsbench-reencrypt` | reencrypt | TLS |

Readiness probes HTTPS `:8443`. Ingress is limited by `08-networkpolicy.yaml` (same namespace + OpenShift ingress).

## 2. Reencrypt destination CA

Patch **before** any reencrypt run. Empty `destinationCACertificate` makes the Route fail or skip pod verification:

```bash
./scripts/patch-reencrypt-ca.sh tlsbench tlsbench tlsbench-reencrypt
oc get route tlsbench-reencrypt -n tlsbench -o jsonpath='{.spec.tls.destinationCACertificate}' | wc -c
```

## 3. Optional write token

Blob write, probe, timings merge, and `/ca.crt` are open unless `BENCH_WRITE_TOKEN` is set. Dashboard reads stay public.

```bash
oc set env -n tlsbench deploy/tlsbench BENCH_WRITE_TOKEN='replace-me'
export BENCH_WRITE_TOKEN=replace-me
```

`vm-bench.sh` sends `Authorization: Bearer` from that env var. Set `SERVE_CA=false` to 404 `/ca.crt`.

Fixed RSA/ECDSA certs: create a Secret and mount it — see [TLS certificates](tls-certificates.md). Extra PEMs in ConfigMap `tlsbench-internal-ca` are merged into `CURL_CA_BUNDLE` by the entrypoint only (not the Go process).

## 4. Dashboard and in-cluster Service

```bash
oc port-forward -n tlsbench svc/tlsbench 8080:8080   # if Routes are not in the browser
```

```bash
oc exec -n tlsbench deploy/tlsbench -- \
  curl -sS -X POST 'http://127.0.0.1:8080/api/blobs?name=1mb.bin&size=1048576'
oc exec -n tlsbench deploy/tlsbench -- \
  sh -c 'head -c 1048576 /dev/urandom | curl -sS --upload-file - \
    http://tlsbench.tlsbench.svc:8080/api/blobs/svc-http.bin'
oc exec -n tlsbench deploy/tlsbench -- \
  sh -c 'head -c 1048576 /dev/urandom | curl -sS --cacert /certs/ca.crt --upload-file - \
    https://tlsbench.tlsbench.svc:8443/api/blobs/svc-https.bin'
```

## 5. External VM → Route

Always pass `--route-mode`. Handshake-only and `--reuse` flags: [methodology](tls-benchmark-methodology.md).

```bash
./scripts/vm-bench.sh --route-mode edge \
  https://tlsbench-edge.apps.example.com 1048576
curl -sk https://tlsbench-passthrough.apps.example.com/ca.crt -o ca.crt
./scripts/vm-bench.sh --route-mode passthrough \
  https://tlsbench-passthrough.apps.example.com 1048576 --cacert ca.crt
./scripts/vm-bench.sh --route-mode reencrypt \
  https://tlsbench-reencrypt.apps.example.com 1048576
```

Debug: `oc exec … -- curl -svk https://tlsbench.tlsbench.svc:8443/api/info` or `nc -vz`. ICMP needs `NET_RAW` (dropped). `GLIBC_2.38` on `ping` means rebuild with `busybox:1.36-uclibc`.

```bash
oc delete -k deploy/openshift
# or: oc delete namespace tlsbench
```
