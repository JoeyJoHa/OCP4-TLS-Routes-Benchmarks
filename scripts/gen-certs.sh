#!/usr/bin/env bash
# Generate a CA + server TLS certificate for benchmark runs.
# Usage: ./scripts/gen-certs.sh <profile> <output-dir> <dns-name> [more-dns...]
# Profiles: ecdsa-p256, ecdsa-p384, rsa-2048, rsa-4096
set -euo pipefail

PROFILE="${1:-}"
OUT_DIR="${2:-}"
DNS_NAMES=("${@:3}")

usage() {
  echo "usage: $0 <profile> <output-dir> <dns-name> [more-dns-names...]" >&2
  echo "profiles: ecdsa-p256, ecdsa-p384, rsa-2048, rsa-4096" >&2
  echo "example: $0 rsa-2048 ./certs/rsa-2048 tlsbench.apps.example.com localhost" >&2
  exit 1
}

[[ -n "$PROFILE" && -n "$OUT_DIR" && ${#DNS_NAMES[@]} -gt 0 ]] || usage

command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }

mkdir -p "$OUT_DIR"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

CA_KEY="$WORKDIR/ca.key"
SRV_KEY="$WORKDIR/tls.key"
CA_CSR="$WORKDIR/ca.csr"
SRV_CSR="$WORKDIR/tls.csr"
SAN_FILE="$WORKDIR/sans.cnf"
CA_EXT="$WORKDIR/ca.ext"

{
  echo "[req]"
  echo "distinguished_name = dn"
  echo "req_extensions = ext"
  echo "[dn]"
  echo "[ext]"
  echo "subjectAltName = @alt_names"
  echo "[alt_names]"
  i=1
  for name in "${DNS_NAMES[@]}"; do
    echo "DNS.$i = $name"
    i=$((i + 1))
  done
} > "$SAN_FILE"

cat > "$CA_EXT" <<'EOF'
basicConstraints = critical, CA:TRUE
keyUsage = critical, keyCertSign, cRLSign
EOF

case "$PROFILE" in
  ecdsa-p256)
    openssl ecparam -genkey -name prime256v1 -out "$CA_KEY"
    openssl ecparam -genkey -name prime256v1 -out "$SRV_KEY"
    ;;
  ecdsa-p384)
    openssl ecparam -genkey -name secp384r1 -out "$CA_KEY"
    openssl ecparam -genkey -name secp384r1 -out "$SRV_KEY"
    ;;
  rsa-2048)
    openssl genrsa -out "$CA_KEY" 2048
    openssl genrsa -out "$SRV_KEY" 2048
    ;;
  rsa-4096)
    openssl genrsa -out "$CA_KEY" 4096
    openssl genrsa -out "$SRV_KEY" 4096
    ;;
  *)
    echo "unknown profile: $PROFILE" >&2
    usage
    ;;
esac

openssl req -new -key "$CA_KEY" -out "$CA_CSR" -subj "/CN=tlsbench-bench-ca/O=OCP4 TLS Bench"
openssl x509 -req -days 365 -in "$CA_CSR" -signkey "$CA_KEY" -out "$OUT_DIR/ca.crt" \
  -extfile "$CA_EXT"

PRIMARY_CN="${DNS_NAMES[0]}"
openssl req -new -key "$SRV_KEY" -out "$SRV_CSR" -subj "/CN=${PRIMARY_CN}/O=OCP4 TLS Bench"
openssl x509 -req -days 365 -in "$SRV_CSR" -CA "$OUT_DIR/ca.crt" -CAkey "$CA_KEY" -CAcreateserial \
  -out "$OUT_DIR/tls.crt" -extfile "$SAN_FILE" -extensions ext

cp "$SRV_KEY" "$OUT_DIR/tls.key"
cp "$CA_KEY" "$OUT_DIR/ca.key"
chmod 600 "$OUT_DIR/tls.key" "$OUT_DIR/ca.key"
chmod 644 "$OUT_DIR/tls.crt" "$OUT_DIR/ca.crt"

echo "Wrote $PROFILE certificates to $OUT_DIR"
echo "  tls.crt / tls.key  — mount as TLS_CERT_FILE / TLS_KEY_FILE"
echo "  ca.crt             — TLS_CA_FILE, GET /ca.crt, reencrypt destination CA"
echo "  ca.key             — keep offline (not for pod mount)"
