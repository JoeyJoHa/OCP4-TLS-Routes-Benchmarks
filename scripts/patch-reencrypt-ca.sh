#!/usr/bin/env bash
# Copy the generated app CA from the pod into a reencrypt Route.
# Usage: ./scripts/patch-reencrypt-ca.sh [namespace] [deployment] [route]
set -euo pipefail

NAMESPACE="${1:-tlsbench}"
DEPLOYMENT="${2:-tlsbench}"
ROUTE="${3:-tlsbench-reencrypt}"

TMP="$(mktemp)"
trap 'rm -f "${TMP}" "${TMP}.patch.yaml"' EXIT

oc exec -n "${NAMESPACE}" "deploy/${DEPLOYMENT}" -- cat /certs/ca.crt > "${TMP}"
if ! grep -q "BEGIN CERTIFICATE" "${TMP}"; then
  echo "could not read a PEM certificate from deploy/${DEPLOYMENT}:/certs/ca.crt" >&2
  exit 1
fi

{
  echo "spec:"
  echo "  tls:"
  echo "    destinationCACertificate: |"
  sed 's/^/      /' "${TMP}"
} > "${TMP}.patch.yaml"

oc patch route "${ROUTE}" -n "${NAMESPACE}" --type merge --patch-file "${TMP}.patch.yaml"
echo "patched route/${ROUTE} destination CA"
