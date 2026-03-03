FROM golang:1.25-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rpa ./cmd/rpa && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o token-refresh ./cmd/token-refresh
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata chromium nss freetype harfbuzz ttf-freefont procps && rm -rf /var/cache/apk/*
RUN addgroup -g 1000 -S rpa && adduser -u 1000 -S rpa -G rpa
WORKDIR /app
COPY --from=builder /app/rpa .
COPY --from=builder /app/token-refresh .
RUN mkdir -p /app/outputs /app/data /home/rpa/.hourglass-rpa && chown -R rpa:rpa /app /home/rpa
USER rpa
ENV TZ=America/Sao_Paulo
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json
VOLUME ["/app/outputs", "/app/data", "/home/rpa/.hourglass-rpa"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 CMD ps aux | grep -v grep | grep -q "rpa" || exit 1
ENTRYPOINT ["./rpa"]
