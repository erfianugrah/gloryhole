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
#
# Dynamic state (policies, ACL, etc.) is stored in SQLite on the persistent
# volume (/var/lib/glory-hole/), NOT in the config file. The config file
# is read-only and only provides static infrastructure settings.

GLORY_USER="glory-hole"
GLORY_UID=1000
GLORY_GID=1000

# Config persistence: on first boot, copy the baked-in config to the persistent
# volume so API changes (local records, forwarding, blocklist sources) survive
# container restarts and deploys. On subsequent boots, the volume copy is used.
BAKED_CONFIG="/etc/glory-hole/config.yml"
LIVE_CONFIG="/var/lib/glory-hole/config.yml"

if [ -f "$BAKED_CONFIG" ] && [ -d "/var/lib/glory-hole" ]; then
    if [ ! -f "$LIVE_CONFIG" ]; then
        echo "glory-hole: first boot — copying config to persistent volume"
        cp "$BAKED_CONFIG" "$LIVE_CONFIG"
    fi
    # Rewrite args: replace the baked config path with the persistent copy.
    # This preserves any other flags the user may have set.
    NEW_ARGS=""
    for arg in "$@"; do
        if [ "$arg" = "$BAKED_CONFIG" ]; then
            NEW_ARGS="$NEW_ARGS $LIVE_CONFIG"
        else
            NEW_ARGS="$NEW_ARGS $arg"
        fi
    done
    set -- $NEW_ARGS
fi

if [ "$(id -u)" = "0" ]; then
    # Fix ownership on all app directories so glory-hole user can read/write.
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
