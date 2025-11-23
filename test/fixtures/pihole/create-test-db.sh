#!/bin/bash
# Create a test Pi-hole gravity.db for testing import functionality

DB_PATH="gravity.db"

# Remove existing database
rm -f "$DB_PATH"

# Create database and tables
sqlite3 "$DB_PATH" <<'EOF'
-- Create adlist table (blocklists)
CREATE TABLE adlist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    address TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    date_added INTEGER NOT NULL DEFAULT (cast(strftime('%s', 'now') as int)),
    date_modified INTEGER NOT NULL DEFAULT (cast(strftime('%s', 'now') as int)),
    comment TEXT
);

-- Create domainlist table (whitelist/blacklist)
CREATE TABLE domainlist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type INTEGER NOT NULL DEFAULT 0,
    domain TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    date_added INTEGER NOT NULL DEFAULT (cast(strftime('%s', 'now') as int)),
    date_modified INTEGER NOT NULL DEFAULT (cast(strftime('%s', 'now') as int)),
    comment TEXT,
    UNIQUE(domain, type)
);

-- Create gravity table (blocked domains from lists)
CREATE TABLE gravity (
    domain TEXT PRIMARY KEY
);

-- Insert test blocklists
INSERT INTO adlist (address, enabled, comment) VALUES
    ('https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts', 1, 'StevenBlack Unified'),
    ('https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/ultimate.txt', 1, 'Hagezi Ultimate');

-- Insert test whitelist patterns
-- Type 0 = whitelist exact, 2 = whitelist regex
INSERT INTO domainlist (type, domain, enabled, comment) VALUES
    (0, 'google.com', 1, 'Allow Google'),
    (0, 'github.com', 1, 'Allow GitHub'),
    (2, '(\.|^)taskassist-pa\.clients6\.google\.com$', 1, 'Google Assistant'),
    (2, '(\.|^)proxy\.cloudflare-gateway\.com$', 1, 'Cloudflare Gateway');

-- Insert test blacklist patterns
-- Type 1 = blacklist exact, 3 = blacklist regex
INSERT INTO domainlist (type, domain, enabled, comment) VALUES
    (1, 'facebook.com', 1, 'Block Facebook'),
    (3, '^ad[sz]\..*\.com$', 1, 'Block ads/adz domains');

-- Insert test blocked domains (simulating downloaded blocklists)
INSERT INTO gravity (domain) VALUES
    ('ads.example.com'),
    ('tracker.example.com'),
    ('malware.example.com'),
    ('doubleclick.net'),
    ('googleads.com');

-- Show what we created
SELECT 'Blocklists:', COUNT(*) FROM adlist WHERE enabled = 1;
SELECT 'Whitelist patterns:', COUNT(*) FROM domainlist WHERE enabled = 1 AND (type = 0 OR type = 2);
SELECT 'Blacklist patterns:', COUNT(*) FROM domainlist WHERE enabled = 1 AND (type = 1 OR type = 3);
SELECT 'Blocked domains:', COUNT(*) FROM gravity;
EOF

echo "Created test database: $DB_PATH"
