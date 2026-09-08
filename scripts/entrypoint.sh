#!/bin/sh
set -eu

BUNDLE="${TLS_CA_BUNDLE_FILE:-/certs/ca-bundle.pem}"
CA_DIR="${TLS_CA_DIR:-/etc/pki/internal-ca}"
CA_FILE="${TLS_CA_FILE:-/certs/ca.crt}"
SYSTEM_BUNDLE="/etc/pki/tls/certs/ca-bundle.crt"

mkdir -p "$(dirname "$BUNDLE")" "$CA_DIR" /certs /data/blobs /data/results

if [ -f "$SYSTEM_BUNDLE" ]; then
  cp "$SYSTEM_BUNDLE" "$BUNDLE"
else
  : > "$BUNDLE"
fi

append_pem() {
  if [ -f "$1" ]; then
    printf '\n' >> "$BUNDLE"
    cat "$1" >> "$BUNDLE"
  fi
}

append_pem "$CA_FILE"

if [ -d "$CA_DIR" ]; then
  for f in "$CA_DIR"/*.crt "$CA_DIR"/*.pem "$CA_DIR"/*.cer; do
    [ -f "$f" ] || continue
    append_pem "$f"
  done
fi

export SSL_CERT_FILE="$BUNDLE"
export CURL_CA_BUNDLE="$BUNDLE"

exec /usr/local/bin/tlsbench
