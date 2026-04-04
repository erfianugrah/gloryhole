# Glory-Hole DNS Server
# Multi-stage Docker build for production deployment

# Stage 1a: Build Unbound from source (runs in parallel with Go build)
FROM alpine:3.21 AS unbound-builder

ARG UNBOUND_VERSION=1.24.2

RUN apk add --no-cache build-base openssl-dev libexpat expat-dev libevent-dev fstrm-dev protobuf-c-dev curl

RUN curl -fsSL "https://nlnetlabs.nl/downloads/unbound/unbound-${UNBOUND_VERSION}.tar.gz" \
        -o unbound.tar.gz && \
    tar xzf unbound.tar.gz

WORKDIR /unbound-${UNBOUND_VERSION}

RUN ./configure \
        --prefix=/opt/unbound \
        --with-libevent \
        --with-ssl \
        --enable-dnstap \
        --disable-flto \
        --without-pythonmodule \
        --without-pyunbound && \
    make -j$(nproc) && \
    make install

# Fetch fresh root hints
RUN curl -fsSL https://www.internic.net/domain/named.root \
        -o /opt/unbound/etc/unbound/root.hints

# Stage 1b: Build Glory-Hole
FROM golang:1.24-alpine AS builder

# Accept build arguments
ARG VERSION=dev
ARG BUILD_TIME=unknown

# Install build dependencies (including npm for Astro dashboard)
RUN apk add --no-cache git ca-certificates tzdata build-base nodejs npm

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build Astro dashboard (outputs to pkg/api/ui/static/dist/)
RUN cd pkg/api/ui/dashboard && npm ci && npm run build

# Build the application
# - Strip debug info (-s -w)
# - Set version from build arg
# - Enable static linking
# - Mount Go build cache for faster rebuilds
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 GOOS=linux go build \
	-ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -extldflags '-static'" \
	-o glory-hole \
	./cmd/glory-hole

# Verify binary
RUN ./glory-hole -version || true

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
# su-exec: drop privileges from root to glory-hole in entrypoint
# libcap: setcap for NET_BIND_SERVICE on the binary (port 53)
RUN apk --no-cache add \
	ca-certificates \
	tzdata \
	sqlite \
	su-exec \
	libcap \
	libevent \
	libexpat \
	fstrm \
	protobuf-c \
	&& rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 glory-hole && \
	adduser -D -u 1000 -G glory-hole glory-hole

# Create necessary directories
RUN mkdir -p /etc/glory-hole /var/lib/glory-hole /var/log/glory-hole && \
	chown -R glory-hole:glory-hole /etc/glory-hole /var/lib/glory-hole /var/log/glory-hole

# Copy Unbound binaries from build stage
COPY --from=unbound-builder /opt/unbound/sbin/unbound /usr/local/bin/unbound
COPY --from=unbound-builder /opt/unbound/sbin/unbound-control /usr/local/bin/unbound-control
COPY --from=unbound-builder /opt/unbound/sbin/unbound-checkconf /usr/local/bin/unbound-checkconf
COPY --from=unbound-builder /opt/unbound/sbin/unbound-anchor /usr/local/bin/unbound-anchor

# Copy libunbound shared library (needed by unbound-anchor for DNSSEC bootstrapping)
COPY --from=unbound-builder /opt/unbound/lib/libunbound.so.8 /usr/lib/libunbound.so.8

# Copy default Unbound config and root hints
COPY deploy/unbound/unbound.conf /etc/unbound/unbound.conf
COPY --from=unbound-builder /opt/unbound/etc/unbound/root.hints /etc/unbound/root.hints

# Create Unbound runtime directories and bootstrap DNSSEC root key
RUN mkdir -p /etc/unbound/custom.conf.d /var/run/unbound && \
	/usr/local/bin/unbound-anchor -a /etc/unbound/root.key || true && \
	chown -R glory-hole:glory-hole /etc/unbound /var/run/unbound

# Copy binary from builder
COPY --from=builder /build/glory-hole /usr/local/bin/glory-hole
RUN chmod +x /usr/local/bin/glory-hole

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Copy configuration examples
COPY --chown=glory-hole:glory-hole config/config.example.yml /etc/glory-hole/config.example.yml

# Do NOT set USER here — the entrypoint starts as root to fix
# mounted volume permissions, then drops to glory-hole via su-exec.
# For Kubernetes, use securityContext to run as UID 1000 directly.

# Set working directory
WORKDIR /var/lib/glory-hole

# Expose ports
# 53/udp - DNS queries
# 53/tcp - DNS queries over TCP
# 853/tcp - DNS-over-TLS (DoT)
# 8080/tcp - HTTP API and Web UI
# 9090/tcp - Prometheus metrics
EXPOSE 53/udp 53/tcp 853/tcp 8080/tcp 9090/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Entrypoint handles privilege drop
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

# Default command (passed as args to entrypoint)
CMD ["-config", "/etc/glory-hole/config.yml"]

# Labels
LABEL org.opencontainers.image.title="Glory-Hole DNS Server"
LABEL org.opencontainers.image.description="High-performance DNS server with ad-blocking, caching, and policy engine"
LABEL org.opencontainers.image.vendor="Glory-Hole"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.source="https://github.com/erfianugrah/glory-hole"
LABEL org.opencontainers.image.documentation="https://github.com/erfianugrah/glory-hole/tree/main/docs"
