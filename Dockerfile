FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o rpa ./cmd/rpa

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata chromium nss freetype harfbuzz ttf-freefont curl && \
    addgroup -g 1000 -S rpa && \
    adduser -u 1000 -S rpa -G rpa
WORKDIR /app
COPY --from=builder /app/rpa ./
RUN mkdir -p /app/outputs /app/data /home/rpa/.hourglass-rpa && \
    chown -R rpa:rpa /app /home/rpa
USER rpa
ENV TZ=America/Sao_Paulo \
    CHROME_BIN=/usr/bin/chromium-browser \
    TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json
VOLUME ["/app/outputs", "/app/data", "/home/rpa/.hourglass-rpa"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=5 \
    CMD ps aux | grep -v grep | grep -q "./rpa" || exit 1
ENTRYPOINT ["./rpa"]
