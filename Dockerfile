# Glory-Hole DNS Server - Multi-stage Dockerfile
# Stage 1: Builder - Compile the Go application
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary
# -ldflags to strip debug info and set version
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o glory-hole \
    ./cmd/glory-hole

# Stage 2: Runtime - Minimal image
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    libcap \
    && addgroup -g 1000 glory-hole \
    && adduser -u 1000 -G glory-hole -s /bin/sh -D glory-hole

# Copy binary from builder
COPY --from=builder /build/glory-hole /usr/local/bin/glory-hole

# Copy example config (users should mount their own)
COPY --from=builder /build/config.example.yml /etc/glory-hole/config.example.yml

# Create directories with proper permissions
RUN mkdir -p /var/lib/glory-hole /var/log/glory-hole /etc/glory-hole \
    && chown -R glory-hole:glory-hole /var/lib/glory-hole /var/log/glory-hole /etc/glory-hole

# Grant capability to bind to privileged ports (port 53)
# This allows non-root user to bind to port 53
RUN setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole

# Switch to non-root user
USER glory-hole

# Set working directory
WORKDIR /var/lib/glory-hole

# Expose ports
# 53/udp - DNS queries
# 53/tcp - DNS queries over TCP
# 8080 - REST API
# 9090 - Prometheus metrics
EXPOSE 53/udp 53/tcp 8080 9090

# Health check using the built-in flag
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/glory-hole", "--health-check"]

# Default command
# Users should mount their config at /etc/glory-hole/config.yml
ENTRYPOINT ["/usr/local/bin/glory-hole"]
CMD ["-config", "/etc/glory-hole/config.yml"]
