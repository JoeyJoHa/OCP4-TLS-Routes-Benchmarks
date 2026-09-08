# Stage 1: build a static Go binary
FROM golang:1.23-bookworm AS builder
WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tlsbench ./cmd/server

# Stage 2: OpenShift-friendly runtime with network tools
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.6

ENV HTTP_ADDR=:8080 \
    HTTPS_ADDR=:8443 \
    TLS_CERT_FILE=/certs/tls.crt \
    TLS_KEY_FILE=/certs/tls.key \
    TLS_CA_FILE=/certs/ca.crt \
    TLS_CA_DIR=/etc/pki/internal-ca \
    TLS_CA_BUNDLE_FILE=/certs/ca-bundle.pem \
    DATA_DIR=/data \
    RESULTS_LOG=/data/results/runs.jsonl \
    SSL_CERT_FILE=/certs/ca-bundle.pem \
    CURL_CA_BUNDLE=/certs/ca-bundle.pem

RUN microdnf -y install --setopt=install_weak_deps=0 \
        iputils traceroute nmap-ncat curl ca-certificates \
    && microdnf clean all \
    && mkdir -p /certs /data /etc/pki/internal-ca \
    && chgrp -R 0 /certs /data /etc/pki/internal-ca \
    && chmod -R g=u /certs /data /etc/pki/internal-ca

COPY --from=builder /out/tlsbench /usr/local/bin/tlsbench
COPY scripts/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/tlsbench /usr/local/bin/entrypoint.sh

EXPOSE 8080 8443

USER 65532:0

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
