-- Initial schema for Glory-Hole query logging and statistics

-- Queries table: stores individual DNS query logs
CREATE TABLE IF NOT EXISTS queries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    client_ip TEXT NOT NULL,
    domain TEXT NOT NULL,
    query_type TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    blocked BOOLEAN NOT NULL,
    cached BOOLEAN NOT NULL,
    response_time_ms INTEGER NOT NULL,
    upstream TEXT,
    upstream_time_ms REAL NOT NULL DEFAULT 0
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_queries_timestamp ON queries(timestamp);
CREATE INDEX IF NOT EXISTS idx_queries_domain ON queries(domain);
CREATE INDEX IF NOT EXISTS idx_queries_blocked ON queries(blocked);
CREATE INDEX IF NOT EXISTS idx_queries_client_ip ON queries(client_ip);
CREATE INDEX IF NOT EXISTS idx_queries_cached ON queries(cached);

-- Statistics table: pre-aggregated statistics by hour
CREATE TABLE IF NOT EXISTS statistics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    hour DATETIME NOT NULL UNIQUE,
    total_queries INTEGER NOT NULL DEFAULT 0,
    blocked_queries INTEGER NOT NULL DEFAULT 0,
    cached_queries INTEGER NOT NULL DEFAULT 0,
    avg_response_time_ms REAL NOT NULL DEFAULT 0,
    unique_domains INTEGER NOT NULL DEFAULT 0,
    unique_clients INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_statistics_hour ON statistics(hour);

-- Domain stats table: tracks query counts per domain
CREATE TABLE IF NOT EXISTS domain_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL UNIQUE,
    query_count INTEGER NOT NULL DEFAULT 1,
    last_queried DATETIME NOT NULL,
    first_queried DATETIME NOT NULL,
    blocked BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_domain_stats_domain ON domain_stats(domain);
CREATE INDEX IF NOT EXISTS idx_domain_stats_count ON domain_stats(query_count DESC);
CREATE INDEX IF NOT EXISTS idx_domain_stats_blocked ON domain_stats(blocked);

-- Schema version table
-- Note: The migration system will insert the version record
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
