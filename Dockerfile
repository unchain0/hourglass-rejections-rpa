# Build stage
FROM golang:1.25-alpine AS builder

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rpa ./cmd/rpa

# Final stage
FROM alpine:latest

# Install ca-certificates, timezone data, and procps for healthcheck
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    procps \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 -S rpa && \
    adduser -u 1000 -S rpa -G rpa

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/rpa .

# Create directories for outputs
RUN mkdir -p /app/outputs /app/data && \
    chown -R rpa:rpa /app

# Switch to non-root user
USER rpa

# Set environment variables
ENV TZ=America/Sao_Paulo
ENV CHROME_BIN=/usr/bin/chromium-browser

# Expose volumes for persistent data
VOLUME ["/app/outputs", "/app/data"]

# Health check - using ps instead of pgrep for non-root compatibility
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ps aux | grep -v grep | grep -q "rpa" || exit 1

# Run the application
ENTRYPOINT ["./rpa"]
