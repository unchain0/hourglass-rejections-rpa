FROM golang:1.26-alpine AS builder

ARG APP_VERSION=dev
ARG VCS_REF=unknown

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s -X main.version=${APP_VERSION} -X main.revision=${VCS_REF}" -o /out/rpa ./cmd/rpa && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o /out/save-tokens ./cmd/save-tokens && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o /out/token-refresh ./cmd/token-refresh && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o /out/setup-auth ./cmd/setup-auth

FROM alpine:3.23

ARG APP_VERSION=dev
ARG VCS_REF=unknown

LABEL org.opencontainers.image.title="Hourglass Rejections RPA" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.source="https://github.com/unchain0/hourglass-rejections-rpa"

RUN apk add --no-cache ca-certificates tzdata chromium nss freetype harfbuzz ttf-freefont curl && \
    addgroup -g 1000 -S rpa && \
    adduser -u 1000 -S rpa -G rpa

WORKDIR /app
COPY --from=builder /out/ ./
RUN mkdir -p /home/rpa/.hourglass-rpa && \
    chown -R rpa:rpa /app /home/rpa

USER rpa
ENV TZ=America/Sao_Paulo \
    CHROME_BIN=/usr/bin/chromium-browser \
    TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json \
    WEBAUTHN_TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json

VOLUME ["/home/rpa/.hourglass-rpa"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=5 \
    CMD pgrep -x rpa >/dev/null && test -s "$WEBAUTHN_TOKENS_PATH" || exit 1
ENTRYPOINT ["./rpa"]
