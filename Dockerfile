# Glory-Hole DNS Server
# Multi-stage Docker build for production deployment

# Stage 1: Build
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata build-base

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build the application
# - Strip debug info (-s -w)
# - Set version from git tag
# - Enable static linking
RUN CGO_ENABLED=1 GOOS=linux go build \
	-a -installsuffix cgo \
	-ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.buildTime=$(date -u '+%Y-%m-%d_%H:%M:%S') -extldflags '-static'" \
	-o glory-hole \
	./cmd/glory-hole

# Verify binary
RUN ./glory-hole -version || true

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add \
	ca-certificates \
	tzdata \
	sqlite \
	&& rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 glory-hole && \
	adduser -D -u 1000 -G glory-hole glory-hole

# Create necessary directories
RUN mkdir -p /etc/glory-hole /var/lib/glory-hole /var/log/glory-hole && \
	chown -R glory-hole:glory-hole /etc/glory-hole /var/lib/glory-hole /var/log/glory-hole

# Copy binary from builder
COPY --from=builder /build/glory-hole /usr/local/bin/glory-hole
RUN chmod +x /usr/local/bin/glory-hole

# Copy configuration examples
COPY --chown=glory-hole:glory-hole config/config.example.yml /etc/glory-hole/config.example.yml

# Switch to non-root user
USER glory-hole

# Set working directory
WORKDIR /var/lib/glory-hole

# Expose ports
# 53/udp - DNS queries
# 53/tcp - DNS queries over TCP
# 8080/tcp - HTTP API and Web UI
# 9090/tcp - Prometheus metrics
EXPOSE 53/udp 53/tcp 8080/tcp 9090/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/glory-hole"]

# Default command
CMD ["-config", "/etc/glory-hole/config.yml"]

# Labels
LABEL org.opencontainers.image.title="Glory-Hole DNS Server"
LABEL org.opencontainers.image.description="High-performance DNS server with ad-blocking, caching, and policy engine"
LABEL org.opencontainers.image.vendor="Glory-Hole"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.source="https://github.com/erfianugrah/glory-hole"
LABEL org.opencontainers.image.documentation="https://github.com/erfianugrah/glory-hole/tree/main/docs"
