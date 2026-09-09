# Running TLS benchmarks on OpenShift

Deploy the app in-cluster with a **PVC**, **Service**, and **Routes**, then run benchmarks from a Linux VM (external) or another pod (in-cluster).

## Prerequisites

- OpenShift 4.x cluster and `oc` logged in
- Image built and pushed to a registry the cluster can pull
- Optional: Linux VM with `curl` and DNS to Route hostnames

## 1. Build and push the image

On your workstation (Podman):

```bash
make image
podman tag tlsbench:dev quay.io/<user>/tlsbench:latest
podman push quay.io/<user>/tlsbench:latest
```

Update the image in `deploy/openshift/03-deployment.yaml`:

```yaml
image: quay.io/<user>/tlsbench:latest
imagePullPolicy: Always
```

## 2. Deploy manifests

```bash
oc apply -k deploy/openshift
oc get pods,svc,route -n tlsbench
```

Wait until the pod is ready:

```bash
oc wait -n tlsbench --for=condition=ready pod -l app.kubernetes.io/name=tlsbench --timeout=120s
```

Routes created by default:

| Route | Termination | Pod sees |
| --- | --- | --- |
| `tlsbench-edge` | edge | HTTP |
| `tlsbench-passthrough` | passthrough | TLS |
| `tlsbench-reencrypt` | reencrypt | TLS |

Get hostnames:

```bash
oc get route -n tlsbench
```

## 3. Patch reencrypt destination CA

After the pod has generated `/certs/ca.crt` (first start with emptyDir certs):

```bash
./scripts/patch-reencrypt-ca.sh tlsbench tlsbench tlsbench-reencrypt
```

## 4. TLS certificates with Secrets (recommended for algorithm benchmarks)

By default the Deployment uses an **emptyDir** for `/certs` and the app **auto-generates** ECDSA P-256 certs on first start.

For **RSA 2048**, **RSA 4096**, or fixed lab certs, create a Secret and mount it instead.

### Generate certs on your workstation

```bash
./scripts/gen-certs.sh rsa-2048 ./certs-rsa2048 tlsbench-passthrough.apps.cluster.example.com
```

### Create the Secret

```bash
oc create secret generic tlsbench-server -n tlsbench \
  --from-file=tls.crt=./certs-rsa2048/tls.crt \
  --from-file=tls.key=./certs-rsa2048/tls.key \
  --from-file=ca.crt=./certs-rsa2048/ca.crt
```

### Patch the Deployment

Add to `deploy/openshift/03-deployment.yaml` (or patch live):

**Environment:**

```yaml
- name: TLS_CERT_FILE
  value: /etc/tls/private/tls.crt
- name: TLS_KEY_FILE
  value: /etc/tls/private/tls.key
- name: TLS_CA_FILE
  value: /etc/tls/private/ca.crt
```

**volumeMounts:**

```yaml
- name: server-tls
  mountPath: /etc/tls/private
  readOnly: true
```

**volumes** (replace or remove `certs` emptyDir if you only use the Secret):

```yaml
- name: server-tls
  secret:
    secretName: tlsbench-server
```

Redeploy:

```bash
oc apply -k deploy/openshift
oc rollout restart deployment/tlsbench -n tlsbench
```

Repeat benchmarks with a new Secret per key type (ecdsa-p256, rsa-2048, rsa-4096). See [TLS certificates for benchmarks](tls-certificates.md).

### Extra internal CAs (pod-to-pod)

Add PEM files to ConfigMap `tlsbench-internal-ca` (see `deploy/openshift/02-configmap-ca.yaml`). The entrypoint merges them into `CURL_CA_BUNDLE` for in-cluster `curl`.

## 5. Open the dashboard

Port-forward if Routes are not reachable from your browser:

```bash
oc port-forward -n tlsbench svc/tlsbench 8080:8080
```

Open http://127.0.0.1:8080/

## 6. In-cluster benchmarks (Service HTTP vs HTTPS)

```bash
# Disk generate
oc exec -n tlsbench deploy/tlsbench -- \
  curl -sS -X POST 'http://127.0.0.1:8080/api/blobs?name=1mb.bin&size=1048576'

# Upload HTTP (Service)
oc exec -n tlsbench deploy/tlsbench -- \
  sh -c 'head -c 1048576 /dev/urandom | curl -sS --upload-file - \
    http://tlsbench.tlsbench.svc:8080/api/blobs/svc-http.bin'

# Upload HTTPS (Service) — TLS decrypt at pod
oc exec -n tlsbench deploy/tlsbench -- \
  sh -c 'head -c 1048576 /dev/urandom | curl -sS --cacert /certs/ca.crt --upload-file - \
    https://tlsbench.tlsbench.svc:8443/api/blobs/svc-https.bin'
```

Check `/api/info` over each path to confirm the `tls` field.

## 7. External benchmarks (Linux VM → Route)

Replace hostnames with your cluster Routes.

```bash
# Edge — TLS terminates at router; pod sees HTTP
./scripts/vm-bench.sh https://tlsbench-edge.apps.example.com 1048576

# Passthrough — TLS to the app
curl -sk https://tlsbench-passthrough.apps.example.com/ca.crt -o ca.crt
./scripts/vm-bench.sh https://tlsbench-passthrough.apps.example.com 1048576 --cacert ca.crt

# Reencrypt — after patch-reencrypt-ca.sh
./scripts/vm-bench.sh https://tlsbench-reencrypt.apps.example.com 1048576
```

## 8. Build comparison tables

Use the same payload size across paths. Example columns:

| Size | Service HTTP upload | Service HTTPS upload | Edge upload | Passthrough upload |
| --- | --- | --- | --- | --- |
| 1 MiB | … | … | … | … |

Data sources:

- Web UI on the Route or port-forward URL
- `GET /api/results` (JSON)
- `curl -w` timings from `scripts/vm-bench.sh`

The **TLS** column in the UI means TLS **at the pod**, not at the client URL scheme alone.

## 9. Debug with network tools

```bash
oc exec -n tlsbench -it deploy/tlsbench -- curl -svk https://tlsbench.tlsbench.svc:8443/api/info
oc exec -n tlsbench -it deploy/tlsbench -- nc -vz tlsbench.tlsbench.svc 8443
```

`ping` / `traceroute` may fail under `restricted-v2` SCC (no `NET_RAW`); `curl` and `nc` still work.

## 10. Cleanup

```bash
oc delete -k deploy/openshift
# or
oc delete namespace tlsbench
```

## Related

- [Running with Podman/Docker](running-with-podman-docker.md) — pre-flight before this guide  
- [TLS certificates for benchmarks](tls-certificates.md) — RSA / ECDSA generation and Secret layout
