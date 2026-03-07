#!/bin/sh
set -e

# Glory-Hole DNS Server - Container Entrypoint
#
# Handles two scenarios:
#   1. Running as root (default image, docker-compose, VyOS/podman):
#      - Ensures mounted config and data dirs are readable/writable by glory-hole user
#      - Drops privileges to glory-hole (UID 1000) via su-exec
#   2. Running as non-root (Kubernetes securityContext, --user flag):
#      - Executes the binary directly

GLORY_USER="glory-hole"
GLORY_UID=1000
GLORY_GID=1000

if [ "$(id -u)" = "0" ]; then
    # Fix ownership on directories the app needs to write to
    chown -R "${GLORY_UID}:${GLORY_GID}" /var/lib/glory-hole /var/log/glory-hole 2>/dev/null || true

    # Ensure mounted config files are readable by the app user
    if [ -d /etc/glory-hole ]; then
        # Make directory traversable
        chmod 755 /etc/glory-hole
        # Make config files readable (preserve read-only mounts by ignoring errors)
        for f in /etc/glory-hole/*.yml /etc/glory-hole/*.yaml; do
            [ -f "$f" ] && chmod 644 "$f" 2>/dev/null || true
        done
    fi

    echo "glory-hole: dropping privileges to ${GLORY_USER} (${GLORY_UID}:${GLORY_GID})"
    exec su-exec "${GLORY_USER}" /usr/local/bin/glory-hole "$@"
fi

# Already running as non-root
exec /usr/local/bin/glory-hole "$@"
