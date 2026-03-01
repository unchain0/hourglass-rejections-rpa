# Build stage
FROM golang:1.22-alpine AS builder

# Install dependencies for Chrome
RUN apk add --no-cache \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    bash

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

# Install Chrome and dependencies
RUN apk add --no-cache \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    tzdata \
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
ENV CHROME_BIN=/usr/bin/chromium-browser \
    CHROME_PATH=/usr/lib/chromium/ \
    DISPLAY=:99

# Expose volume for outputs
VOLUME ["/app/outputs"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep -x rpa >/dev/null || exit 1

# Run the application
ENTRYPOINT ["./rpa"]
