package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const defaultClientPageSize = 50

// GetClientSummaries aggregates per-client statistics with optional pagination.
func (s *SQLiteStorage) GetClientSummaries(ctx context.Context, limit, offset int) ([]*ClientSummary, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}

	if limit <= 0 {
		limit = defaultClientPageSize
	}
	if offset < 0 {
		offset = 0
	}

	const query = `
		WITH aggregated AS (
			SELECT
				client_ip,
				MIN(timestamp) AS first_seen,
				MAX(timestamp) AS last_seen,
				COUNT(*) AS total_queries,
				SUM(CASE WHEN blocked = 1 THEN 1 ELSE 0 END) AS blocked_queries,
				SUM(CASE WHEN response_code = 3 THEN 1 ELSE 0 END) AS nxdomain_queries
			FROM queries
			GROUP BY client_ip
		)
		SELECT
			a.client_ip,
			COALESCE(p.display_name, a.client_ip) AS display_name,
			COALESCE(p.notes, '') AS notes,
			p.group_name,
			COALESCE(g.color, '') AS group_color,
			a.first_seen,
			a.last_seen,
			a.total_queries,
			a.blocked_queries,
			a.nxdomain_queries
		FROM aggregated a
		LEFT JOIN client_profiles p ON p.client_ip = a.client_ip
		LEFT JOIN client_groups g ON p.group_name = g.name
		ORDER BY a.last_seen DESC
		LIMIT ? OFFSET ?;
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query clients failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var clients []*ClientSummary
	for rows.Next() {
		var summary ClientSummary
		var firstSeenStr, lastSeenStr string
		var notes string
		var groupName sql.NullString
		var groupColor sql.NullString
		if err := rows.Scan(
			&summary.ClientIP,
			&summary.DisplayName,
			&notes,
			&groupName,
			&groupColor,
			&firstSeenStr,
			&lastSeenStr,
			&summary.TotalQueries,
			&summary.BlockedQueries,
			&summary.NXDomainCount,
		); err != nil {
			return nil, fmt.Errorf("scan client summary failed: %w", err)
		}
		summary.Notes = notes
		if groupName.Valid {
			summary.GroupName = groupName.String
		}
		if groupColor.Valid {
			summary.GroupColor = groupColor.String
		}
		summary.FirstSeen = parseSQLiteTime(firstSeenStr)
		summary.LastSeen = parseSQLiteTime(lastSeenStr)
		clients = append(clients, &summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate client summaries failed: %w", err)
	}

	return clients, nil
}

// UpdateClientProfile upserts operator-provided metadata for a client.
func (s *SQLiteStorage) UpdateClientProfile(ctx context.Context, profile *ClientProfile) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	if profile == nil {
		return fmt.Errorf("profile cannot be nil")
	}

	groupName := strings.TrimSpace(profile.GroupName)
	if groupName == "" {
		groupName = ""
	}

	const statement = `
		INSERT INTO client_profiles (client_ip, display_name, notes, group_name, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(client_ip) DO UPDATE SET
			display_name = excluded.display_name,
			notes = excluded.notes,
			group_name = excluded.group_name,
			updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := s.db.ExecContext(ctx, statement,
		profile.ClientIP,
		nullify(profile.DisplayName),
		nullify(profile.Notes),
		groupName,
	); err != nil {
		return fmt.Errorf("update client profile failed: %w", err)
	}

	return nil
}

// GetClientGroups returns the configured client groups.
func (s *SQLiteStorage) GetClientGroups(ctx context.Context) ([]*ClientGroup, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}

	const statement = `
		SELECT name, COALESCE(description, ''), COALESCE(color, '')
		FROM client_groups
		ORDER BY name ASC;
	`

	rows, err := s.db.QueryContext(ctx, statement)
	if err != nil {
		return nil, fmt.Errorf("query client groups failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []*ClientGroup
	for rows.Next() {
		var group ClientGroup
		if err := rows.Scan(&group.Name, &group.Description, &group.Color); err != nil {
			return nil, fmt.Errorf("scan client group failed: %w", err)
		}
		groups = append(groups, &group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate client groups failed: %w", err)
	}

	return groups, nil
}

// UpsertClientGroup creates or updates a client group.
func (s *SQLiteStorage) UpsertClientGroup(ctx context.Context, group *ClientGroup) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	if group == nil {
		return fmt.Errorf("group cannot be nil")
	}

	const statement = `
		INSERT INTO client_groups (name, description, color, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, ''), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			description = excluded.description,
			color = excluded.color,
			updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := s.db.ExecContext(ctx, statement, group.Name, nullify(group.Description), nullify(group.Color)); err != nil {
		return fmt.Errorf("upsert client group failed: %w", err)
	}
	return nil
}

// DeleteClientGroup removes a client group and clears associated profiles.
func (s *SQLiteStorage) DeleteClientGroup(ctx context.Context, name string) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE client_profiles SET group_name = NULL WHERE group_name = ?`, name); err != nil {
		return fmt.Errorf("clear client group references failed: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM client_groups WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete client group failed: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit client group delete failed: %w", err)
	}
	return nil
}

func nullify(value string) any {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return v
}

func parseSQLiteTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts
		}
	}
	return time.Time{}
}
