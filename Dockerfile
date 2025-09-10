# Build stage
FROM golang:1.23-alpine AS builder

# Install git and ca-certificates (no longer need build-base for CGO)
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary (pure Go, no CGO needed)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=docker -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -a -installsuffix netgo \
    -o endpoint_forwarder .

# Final stage
FROM alpine:latest

# Install ca-certificates, curl and tzdata for runtime (no longer need sqlite)
RUN apk --no-cache add ca-certificates tzdata curl

# Set timezone to China Standard Time (CST +08:00)
ENV TZ=Asia/Shanghai
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

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
RUN mkdir -p /app/logs /app/config /app/data && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports - 8088 for API/proxy, 8010 for web interface
EXPOSE 8088 8010

# Health check - check application health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:8010/health || exit 1

# Set entrypoint
ENTRYPOINT ["/app/endpoint_forwarder"]

# Default command (can be overridden)
CMD ["-config", "/app/config/config.yaml"]