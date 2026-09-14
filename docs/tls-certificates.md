# TLS certificates for benchmarks

How server **key type** is supplied. Handshake vs bulk comparison rules: [methodology](tls-benchmark-methodology.md).

| Profile | Typical use |
| --- | --- |
| `ecdsa-p256` | Default auto-generated |
| `rsa-2048` | Common legacy default |
| `rsa-4096` | Slower RSA sign |
| `ecdsa-p384` | Stronger ECDSA curve |

Cipher and TLS version are negotiated per connection (`/api/info`: `tls_version`, `cipher`). Names are Go IANA (`TLS_AES_128_GCM_SHA256`), not OpenSSL TLS 1.2 names.

## Generate (`scripts/gen-certs.sh`)

```bash
./scripts/gen-certs.sh <profile> <output-dir> <dns-name> [more-dns-names...]
./scripts/gen-certs.sh ecdsa-p256 ./certs/ecdsa-p256 localhost
./scripts/gen-certs.sh rsa-2048 ./certs/rsa-2048 tlsbench-passthrough.apps.example.com localhost
```

Write under `./certs/` (gitignored). SANs must include every client hostname (Route FQDN, Service DNS, `localhost`).

| File | Role |
| --- | --- |
| `ca.crt` | Trust for `--cacert` and reencrypt `destinationCACertificate` |
| `ca.key` | From `gen-certs.sh` only — keep offline; not mounted in the pod |
| `tls.crt` / `tls.key` | Server cert and key |

The in-app generator writes `ca.crt`, `tls.crt`, and `tls.key` only (no CA private key). Use `gen-certs.sh` when you need a reusable CA key. Do not commit `*.key`.

If `TLS_CERT_FILE` and `TLS_KEY_FILE` are missing, the process writes **ECDSA P-256** to those paths. On OpenShift emptyDir `/certs`, that lasts for the pod lifetime only.

## Podman: mount a profile

```bash
podman run --rm -d --name tlsbench-bench \
  -p 8080:8080 -p 8443:8443 \
  -v tlsbench-data:/data \
  -v "$(pwd)/certs/rsa-2048:/certs:ro" \
  -e TLS_CERT_FILE=/certs/tls.crt \
  -e TLS_KEY_FILE=/certs/tls.key \
  -e TLS_CA_FILE=/certs/ca.crt \
  -e TLS_DNS_NAMES=localhost \
  tlsbench:dev
curl --cacert ./certs/rsa-2048/ca.crt https://127.0.0.1:8443/api/info
```

Swap only the mounted directory between profiles.

## OpenShift: TLS Secret

```bash
oc create secret generic tlsbench-server-rsa2048 -n tlsbench \
  --from-file=tls.crt=./certs/rsa-2048/tls.crt \
  --from-file=tls.key=./certs/rsa-2048/tls.key \
  --from-file=ca.crt=./certs/rsa-2048/ca.crt \
  --dry-run=client -o yaml | oc apply -f -
```

Mount at `/etc/tls/private` (read-only) and set `TLS_CERT_FILE` / `TLS_KEY_FILE` / `TLS_CA_FILE` to those paths. Remove the `certs` emptyDir when the Secret replaces auto-generation. After a new CA, re-run `./scripts/patch-reencrypt-ca.sh tlsbench tlsbench tlsbench-reencrypt`.

External trust: `curl -sk https://tlsbench-passthrough.apps.example.com/ca.crt -o ca.crt` (or the generated `ca.crt`).
