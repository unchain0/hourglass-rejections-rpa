FROM golang:1.26.9-alpine3.23@sha256:6750308561953d451210ed1a8086d36c8c8bdcbf672d38d9d0d424b8a7a0216c AS builder

ARG APP_VERSION=dev
ARG VCS_REF=unknown

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOMAXPROCS=1 \
    GOMEMLIMIT=320MiB

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# Compose runtime surface only. Host CLIs stay on `make build`.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -p=1 \
        -trimpath \
        -ldflags="-w -s -X main.version=${APP_VERSION} -X main.revision=${VCS_REF}" \
        -o /out/rpa \
        ./cmd/rpa && \
    go build -p=1 \
        -trimpath \
        -ldflags="-w -s" \
        -o /out/token-refresh \
        ./cmd/token-refresh

FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

ARG APP_VERSION=dev
ARG VCS_REF=unknown

LABEL org.opencontainers.image.title="Hourglass Rejections RPA" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.source="https://github.com/unchain0/hourglass-rejections-rpa"

# No Chromium: headless renews via WebAuthn. Bootstrap is host-side setup-auth.
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -g 1000 -S rpa && \
    adduser -u 1000 -S rpa -G rpa && \
    mkdir -p /home/rpa/.hourglass-rpa && \
    chmod 700 /home/rpa/.hourglass-rpa && \
    chown -R rpa:rpa /home/rpa

WORKDIR /app
COPY --from=builder --chown=rpa:rpa /out/ ./

USER rpa
ENV TZ=America/Sao_Paulo \
    TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json \
    WEBAUTHN_TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json

VOLUME ["/home/rpa/.hourglass-rpa"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=5 \
    CMD test "$(cat /proc/1/comm)" = "rpa"
ENTRYPOINT ["./rpa"]
