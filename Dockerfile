# Stage 1: static Go binary
FROM docker.io/library/golang:1.23-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY web ./web

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tlsbench ./cmd/server

# musl/uclibc BusyBox is statically linked. The default glibc tag on aarch64
# needs GLIBC_2.38, which Debian bookworm (runtime) does not provide.
FROM docker.io/library/busybox:1.36-uclibc AS tools

FROM docker.io/library/debian:bookworm-slim

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

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/tlsbench /usr/local/bin/tlsbench
COPY --from=tools /bin/busybox /usr/local/bin/busybox
COPY scripts/entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod 0755 /usr/local/bin/tlsbench /usr/local/bin/entrypoint.sh /usr/local/bin/busybox \
    && ln -sf /usr/local/bin/busybox /usr/local/bin/ping \
    && ln -sf /usr/local/bin/busybox /usr/local/bin/traceroute \
    && ln -sf /usr/local/bin/busybox /usr/local/bin/nc \
    && mkdir -p /certs /data /etc/pki/internal-ca \
    && chgrp -R 0 /certs /data /etc/pki/internal-ca \
    && chmod -R g=u /certs /data /etc/pki/internal-ca

EXPOSE 8080 8443

USER 65532:0

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
