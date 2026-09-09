# TLS certificates for benchmarks

Use different **key algorithms and sizes** to measure how server certificate type affects TLS handshake and encrypt/decrypt cost.

| Profile | Typical use |
| --- | --- |
| `ecdsa-p256` | Default auto-generated; fast, modern |
| `rsa-2048` | Common legacy default |
| `rsa-4096` | Stronger RSA; slower handshake/sign |
| `ecdsa-p384` | Stronger ECDSA curve |

Cipher suite and TLS version are negotiated per connection; check `/api/info` for `tls_version` and `cipher`.

## Generate cert sets (workstation)

Use `scripts/gen-certs.sh`:

```bash
./scripts/gen-certs.sh <profile> <output-dir> <dns-name> [more-dns-names...]
```

Examples:

```bash
./scripts/gen-certs.sh ecdsa-p256 ./certs-ecdsa localhost
./scripts/gen-certs.sh rsa-2048 ./certs-rsa2048 tlsbench-passthrough.apps.example.com localhost
./scripts/gen-certs.sh rsa-4096 ./certs-rsa4096 tlsbench-passthrough.apps.example.com
./scripts/gen-certs.sh ecdsa-p384 ./certs-ecdsa384 localhost
```

Output files:

```
ca.crt      # CA certificate (for --cacert and reencrypt destinationCACertificate)
ca.key      # CA private key (keep offline; not mounted in pod)
tls.crt     # Server certificate
tls.key     # Server private key
```

**SANs:** include every hostname clients use (Route FQDN, Service DNS, `localhost` for Podman).

## Podman / Docker — mount as a volume

```bash
podman run --rm -d --name tlsbench-bench \
  -p 8080:8080 -p 8443:8443 \
  -v tlsbench-data:/data \
  -v "$(pwd)/certs-rsa2048:/certs:ro" \
  -e TLS_CERT_FILE=/certs/tls.crt \
  -e TLS_KEY_FILE=/certs/tls.key \
  -e TLS_CA_FILE=/certs/ca.crt \
  -e TLS_DNS_NAMES=localhost \
  tlsbench:dev
```

Run the same upload/download benchmark for each profile; swap only the mounted cert directory.

Trust from the client:

```bash
curl --cacert ./certs-rsa2048/ca.crt https://127.0.0.1:8443/api/info
```

## OpenShift — TLS Secret

### Create Secret from generated files

```bash
oc create secret generic tlsbench-server-rsa2048 -n tlsbench \
  --from-file=tls.crt=./certs-rsa2048/tls.crt \
  --from-file=tls.key=./certs-rsa2048/tls.key \
  --from-file=ca.crt=./certs-rsa2048/ca.crt \
  --dry-run=client -o yaml | oc apply -f -
```

### Mount in Deployment

```yaml
env:
  - name: TLS_CERT_FILE
    value: /etc/tls/private/tls.crt
  - name: TLS_KEY_FILE
    value: /etc/tls/private/tls.key
  - name: TLS_CA_FILE
    value: /etc/tls/private/ca.crt
volumeMounts:
  - name: server-tls
    mountPath: /etc/tls/private
    readOnly: true
volumes:
  - name: server-tls
    secret:
      secretName: tlsbench-server-rsa2048
```

Remove or stop using the `certs` emptyDir volume when the Secret fully replaces auto-generation.

### Reencrypt Route

Set `destinationCACertificate` to the **ca.crt** from the same profile:

```bash
./scripts/patch-reencrypt-ca.sh tlsbench tlsbench tlsbench-reencrypt
```

(After mounting the Secret, copy `ca.crt` from the Secret or re-run patch after exec into the pod.)

### External VM trust

```bash
curl -sk https://tlsbench-passthrough.apps.example.com/ca.crt -o ca.crt
# or use the ca.crt from your gen-certs output directory
curl --cacert ca.crt https://tlsbench-passthrough.apps.example.com/api/info
```

## Benchmark matrix (example)

For each cert profile, run at 1 KiB, 1 MiB, and 10 MiB:

1. Upload HTTP (edge Route or Service :8080)  
2. Upload HTTPS (passthrough or Service :8443)  
3. Download HTTP  
4. Download HTTPS  

Record `tls_handshake_ms`, `transfer_ms`, `total_ms`, and `throughput_mib_s` from the UI or `/api/results`. Handshake cost tracks cert key algorithm (RSA 2048/4096 vs ECDSA); transfer and throughput track the negotiated cipher.

## Auto-generated certs (no Secret)

If `TLS_CERT_FILE` and `TLS_KEY_FILE` are missing, the app writes **ECDSA P-256** to the configured paths on first start. On OpenShift with emptyDir `/certs`, certs persist for the pod lifetime only.

Do not commit `*.key` or `*.pem` private keys to git.
