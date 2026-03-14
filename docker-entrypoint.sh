#!/bin/sh
set -e

# Glory-Hole DNS Server - Container Entrypoint
#
# Handles two scenarios:
#   1. Running as root (default image, docker-compose, VyOS/podman):
#      - Ensures mounted config and data dirs are readable/writable by glory-hole user
#      - Sets NET_BIND_SERVICE capability on the binary so it can bind port 53
#      - Drops privileges to glory-hole (UID 1000) via su-exec
#   2. Running as non-root (Kubernetes securityContext, --user flag):
#      - Executes the binary directly

GLORY_USER="glory-hole"
GLORY_UID=1000
GLORY_GID=1000

if [ "$(id -u)" = "0" ]; then
    # Fix ownership on all app directories so glory-hole user can read/write.
    # /etc/glory-hole needs write access for config persistence (policy saves
    # write a .tmp file then rename).
    chown -R "${GLORY_UID}:${GLORY_GID}" \
        /etc/glory-hole \
        /var/lib/glory-hole \
        /var/log/glory-hole \
        2>/dev/null || true

    # Fix ownership on Unbound directories (config, runtime socket, root key)
    chown -R "${GLORY_UID}:${GLORY_GID}" \
        /etc/unbound \
        /var/run/unbound \
        2>/dev/null || true

    # Grant NET_BIND_SERVICE on the binary so it can bind port 53 after dropping root
    setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole 2>/dev/null || true

    echo "glory-hole: dropping privileges to ${GLORY_USER} (${GLORY_UID}:${GLORY_GID})"
    exec su-exec "${GLORY_USER}" /usr/local/bin/glory-hole "$@"
fi

# Already running as non-root
exec /usr/local/bin/glory-hole "$@"
