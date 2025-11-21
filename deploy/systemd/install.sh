#!/bin/bash
# Glory-Hole DNS Server - systemd Installation Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Usage: sudo ./install.sh"
    exit 1
fi

echo -e "${GREEN}Glory-Hole DNS Server - systemd Installation${NC}"
echo "=============================================="
echo

# 1. Create glory-hole user and group
echo -e "${YELLOW}Creating glory-hole user and group...${NC}"
if ! id -u glory-hole > /dev/null 2>&1; then
    useradd --system --user-group --no-create-home --shell /bin/false glory-hole
    echo -e "${GREEN}✓${NC} User and group created"
else
    echo -e "${GREEN}✓${NC} User already exists"
fi

# 2. Create necessary directories
echo -e "${YELLOW}Creating directories...${NC}"
mkdir -p /etc/glory-hole
mkdir -p /var/lib/glory-hole
mkdir -p /var/log/glory-hole

echo -e "${GREEN}✓${NC} Directories created"

# 3. Set permissions
echo -e "${YELLOW}Setting permissions...${NC}"
chown -R glory-hole:glory-hole /etc/glory-hole
chown -R glory-hole:glory-hole /var/lib/glory-hole
chown -R glory-hole:glory-hole /var/log/glory-hole

chmod 755 /etc/glory-hole
chmod 755 /var/lib/glory-hole
chmod 755 /var/log/glory-hole

echo -e "${GREEN}✓${NC} Permissions set"

# 4. Copy binary (if provided)
if [ -f "../../glory-hole" ]; then
    echo -e "${YELLOW}Installing binary...${NC}"
    cp ../../glory-hole /usr/local/bin/glory-hole
    chmod 755 /usr/local/bin/glory-hole

    # Set capability for port 53 binding
    setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole
    echo -e "${GREEN}✓${NC} Binary installed with CAP_NET_BIND_SERVICE capability"
else
    echo -e "${YELLOW}⚠${NC}  Binary not found at ../../glory-hole"
    echo "    Please copy the glory-hole binary to /usr/local/bin/ manually"
    echo "    And run: sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole"
fi

# 5. Copy example config (if provided)
if [ -f "../../config.example.yml" ] && [ ! -f "/etc/glory-hole/config.yml" ]; then
    echo -e "${YELLOW}Installing example config...${NC}"
    cp ../../config.example.yml /etc/glory-hole/config.yml
    chown glory-hole:glory-hole /etc/glory-hole/config.yml
    chmod 644 /etc/glory-hole/config.yml
    echo -e "${GREEN}✓${NC} Config installed at /etc/glory-hole/config.yml"
    echo -e "${YELLOW}⚠${NC}  Please edit /etc/glory-hole/config.yml before starting the service"
else
    echo -e "${YELLOW}⚠${NC}  Please create /etc/glory-hole/config.yml before starting the service"
fi

# 6. Install systemd service
echo -e "${YELLOW}Installing systemd service...${NC}"
cp glory-hole.service /etc/systemd/system/glory-hole.service
chmod 644 /etc/systemd/system/glory-hole.service

# Reload systemd
systemctl daemon-reload
echo -e "${GREEN}✓${NC} Systemd service installed"

echo
echo -e "${GREEN}Installation complete!${NC}"
echo
echo "Next steps:"
echo "1. Edit the configuration: sudo nano /etc/glory-hole/config.yml"
echo "2. Enable the service: sudo systemctl enable glory-hole"
echo "3. Start the service: sudo systemctl start glory-hole"
echo "4. Check status: sudo systemctl status glory-hole"
echo "5. View logs: sudo journalctl -u glory-hole -f"
echo
