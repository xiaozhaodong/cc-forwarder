# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates (needed for fetching dependencies)
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=docker -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -a -installsuffix cgo \
    -o endpoint_forwarder .

# Final stage
FROM alpine:latest

# Install ca-certificates and curl for HTTPS requests and health checks
RUN apk --no-cache add ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /build/endpoint_forwarder /app/endpoint_forwarder

# Copy configuration files
COPY --from=builder /build/config/example.yaml /app/config/example.yaml

# Copy web static files
COPY --from=builder /build/internal/web/static /app/internal/web/static

# Create necessary directories and set permissions
RUN mkdir -p /app/logs /app/config && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports - 8087 for proxy, 8088 for web interface
EXPOSE 8087 8088

# Health check - use proxy port for health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:8087/health || exit 1

# Set entrypoint
ENTRYPOINT ["/app/endpoint_forwarder"]

# Default command (can be overridden)
CMD ["-config", "/app/config/config.yaml"]