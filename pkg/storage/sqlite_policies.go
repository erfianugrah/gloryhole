package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GetPolicyRules returns all policy rules ordered by sort_order.
func (s *SQLiteStorage) GetPolicyRules(ctx context.Context) ([]*PolicyRule, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}

	ctx, cancel := withQueryTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, logic, action, action_data, enabled, sort_order
		FROM policy_rules
		ORDER BY sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query policy_rules: %w", err)
	}
	defer rows.Close()

	var rules []*PolicyRule
	for rows.Next() {
		r := &PolicyRule{}
		if err := rows.Scan(&r.ID, &r.Name, &r.Logic, &r.Action, &r.ActionData, &r.Enabled, &r.SortOrder); err != nil {
			return nil, fmt.Errorf("scan policy_rules row: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// CreatePolicyRule inserts a new policy rule and returns its auto-generated ID.
func (s *SQLiteStorage) CreatePolicyRule(ctx context.Context, rule *PolicyRule) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrClosed
	}

	ctx, cancel := withQueryTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_rules (name, logic, action, action_data, enabled, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, rule.Name, rule.Logic, rule.Action, rule.ActionData, rule.Enabled, rule.SortOrder)
	if err != nil {
		return 0, fmt.Errorf("insert policy_rules: %w", err)
	}
	return result.LastInsertId()
}

// UpdatePolicyRule updates an existing policy rule by ID.
func (s *SQLiteStorage) UpdatePolicyRule(ctx context.Context, id int64, rule *PolicyRule) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}

	ctx, cancel := withQueryTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		UPDATE policy_rules
		SET name = ?, logic = ?, action = ?, action_data = ?, enabled = ?, sort_order = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, rule.Name, rule.Logic, rule.Action, rule.ActionData, rule.Enabled, rule.SortOrder, id)
	if err != nil {
		return fmt.Errorf("update policy_rules: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("policy rule %d not found", id)
	}
	return nil
}

// DeletePolicyRule removes a policy rule by ID.
func (s *SQLiteStorage) DeletePolicyRule(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}

	ctx, cancel := withQueryTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `DELETE FROM policy_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete policy_rules: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("policy rule %d not found", id)
	}
	return nil
}

// GetDynamicConfig retrieves a value from the dynamic_config key-value store.
// Returns empty string and sql.ErrNoRows if the key doesn't exist.
func (s *SQLiteStorage) GetDynamicConfig(ctx context.Context, key string) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrClosed
	}

	ctx, cancel := withQueryTimeout(ctx, 5*time.Second)
	defer cancel()

	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM dynamic_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get dynamic_config %q: %w", key, err)
	}
	return value, nil
}

// SetDynamicConfig upserts a key-value pair in the dynamic_config table.
func (s *SQLiteStorage) SetDynamicConfig(ctx context.Context, key, value string) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}

	ctx, cancel := withQueryTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dynamic_config (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, value)
	if err != nil {
		return fmt.Errorf("set dynamic_config %q: %w", key, err)
	}
	return nil
}
