# Build stage
FROM golang:1.24-alpine AS builder

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

# Install ca-certificates and timezone data
RUN apk add --no-cache \
    ca-certificates \
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
ENV TZ=America/Sao_Paulo

# Expose volume for outputs
VOLUME ["/app/outputs"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep -x rpa >/dev/null || exit 1

# Run the application
ENTRYPOINT ["./rpa"]
