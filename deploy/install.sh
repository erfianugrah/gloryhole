#!/bin/bash
set -e

# Glory-Hole DNS Server Installation Script
# This script installs Glory-Hole DNS Server system-wide

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/glory-hole"
DATA_DIR="/var/lib/glory-hole"
SERVICE_FILE="/etc/systemd/system/glory-hole.service"
USER="glory-hole"
GROUP="glory-hole"

# Functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root"
        exit 1
    fi
}

check_dependencies() {
    print_info "Checking dependencies..."

    if ! command -v systemctl &> /dev/null; then
        print_error "systemd is required but not installed"
        exit 1
    fi
}

create_user() {
    if id "$USER" &>/dev/null; then
        print_info "User $USER already exists"
    else
        print_info "Creating system user $USER..."
        useradd --system --no-create-home --shell /bin/false "$USER"
    fi
}

build_binary() {
    print_info "Building glory-hole binary..."

    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go 1.24+ and try again"
        exit 1
    fi

    go build -ldflags="-s -w" -o glory-hole ./cmd/glory-hole
}

install_binary() {
    print_info "Installing binary to $INSTALL_DIR..."
    cp glory-hole "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/glory-hole"
    chown root:root "$INSTALL_DIR/glory-hole"
}

install_config() {
    print_info "Creating configuration directory..."
    mkdir -p "$CONFIG_DIR"

    if [[ ! -f "$CONFIG_DIR/config.yml" ]]; then
        print_info "Installing default configuration..."
        cp config.example.yml "$CONFIG_DIR/config.yml"
        chmod 644 "$CONFIG_DIR/config.yml"
        chown root:root "$CONFIG_DIR/config.yml"
        print_warn "Edit $CONFIG_DIR/config.yml before starting the service"
    else
        print_info "Configuration file already exists, skipping..."
    fi
}

create_data_dir() {
    print_info "Creating data directory..."
    mkdir -p "$DATA_DIR"
    chown "$USER:$GROUP" "$DATA_DIR"
    chmod 755 "$DATA_DIR"
}

install_service() {
    print_info "Installing systemd service..."
    cp deploy/systemd/glory-hole.service "$SERVICE_FILE"
    chmod 644 "$SERVICE_FILE"
    systemctl daemon-reload
}

set_capabilities() {
    print_info "Setting capabilities for port 53 binding..."
    setcap 'cap_net_bind_service=+ep' "$INSTALL_DIR/glory-hole"
}

# Main installation
main() {
    print_info "Starting Glory-Hole DNS Server installation..."
    echo

    check_root
    check_dependencies
    create_user
    build_binary
    install_binary
    install_config
    create_data_dir
    install_service
    set_capabilities

    echo
    print_info "Installation complete!"
    echo
    print_info "Next steps:"
    echo "  1. Edit configuration: sudo nano $CONFIG_DIR/config.yml"
    echo "  2. Enable service: sudo systemctl enable glory-hole"
    echo "  3. Start service: sudo systemctl start glory-hole"
    echo "  4. Check status: sudo systemctl status glory-hole"
    echo "  5. View logs: sudo journalctl -u glory-hole -f"
    echo
    print_info "Web UI will be available at http://localhost:8080"
    print_info "Prometheus metrics at http://localhost:9090/metrics"
}

# Run main function
main
