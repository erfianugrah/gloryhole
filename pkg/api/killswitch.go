package api

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// KillSwitchManager manages temporary (duration-based) kill-switches
// that auto-re-enable after a specified duration, similar to Pi-hole's
// "Disable for 5 minutes" feature.
//
// This is separate from the permanent enable/disable flags in the config.
// Temporary disables take precedence over permanent enables.
type KillSwitchManager struct {
	logger *slog.Logger
	stopChan chan struct{}
	blocklistDisabledUntil time.Time
	policiesDisabledUntil  time.Time
	mu sync.RWMutex
	wg       sync.WaitGroup
}

// NewKillSwitchManager creates a new kill-switch manager
func NewKillSwitchManager(logger *slog.Logger) *KillSwitchManager {
	return &KillSwitchManager{
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start begins the background worker that monitors expiration times
func (k *KillSwitchManager) Start(ctx context.Context) {
	k.wg.Add(1)
	go k.monitorExpiration(ctx)
}

// Stop gracefully stops the kill-switch manager
func (k *KillSwitchManager) Stop() {
	close(k.stopChan)
	k.wg.Wait()
}

// monitorExpiration runs a background worker that logs when features auto-re-enable
func (k *KillSwitchManager) monitorExpiration(ctx context.Context) {
	defer k.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var blocklistLogged, policiesLogged bool

	for {
		select {
		case <-ctx.Done():
			return
		case <-k.stopChan:
			return
		case <-ticker.C:
			now := time.Now()

			k.mu.RLock()
			blocklistDisabled := now.Before(k.blocklistDisabledUntil)
			policiesDisabled := now.Before(k.policiesDisabledUntil)
			k.mu.RUnlock()

			// Log when blocklist auto-re-enables
			if !blocklistDisabled && !blocklistLogged {
				k.mu.RLock()
				wasDisabled := !k.blocklistDisabledUntil.IsZero()
				k.mu.RUnlock()

				if wasDisabled {
					k.logger.Info("Blocklist auto-re-enabled after temporary disable")
					blocklistLogged = true
				}
			}

			// Log when policies auto-re-enable
			if !policiesDisabled && !policiesLogged {
				k.mu.RLock()
				wasDisabled := !k.policiesDisabledUntil.IsZero()
				k.mu.RUnlock()

				if wasDisabled {
					k.logger.Info("Policies auto-re-enabled after temporary disable")
					policiesLogged = true
				}
			}

			// Reset logging flags when features are disabled again
			if blocklistDisabled {
				blocklistLogged = false
			}
			if policiesDisabled {
				policiesLogged = false
			}
		}
	}
}

// DisableBlocklistFor temporarily disables the blocklist for the specified duration
func (k *KillSwitchManager) DisableBlocklistFor(duration time.Duration) time.Time {
	k.mu.Lock()
	defer k.mu.Unlock()

	until := time.Now().Add(duration)
	k.blocklistDisabledUntil = until

	k.logger.Warn("Blocklist temporarily disabled",
		"duration", duration,
		"until", until)

	return until
}

// DisablePoliciesFor temporarily disables policies for the specified duration
func (k *KillSwitchManager) DisablePoliciesFor(duration time.Duration) time.Time {
	k.mu.Lock()
	defer k.mu.Unlock()

	until := time.Now().Add(duration)
	k.policiesDisabledUntil = until

	k.logger.Warn("Policies temporarily disabled",
		"duration", duration,
		"until", until)

	return until
}

// EnableBlocklist immediately re-enables the blocklist (cancels temporary disable)
func (k *KillSwitchManager) EnableBlocklist() {
	k.mu.Lock()
	defer k.mu.Unlock()

	wasDisabled := time.Now().Before(k.blocklistDisabledUntil)
	k.blocklistDisabledUntil = time.Time{} // Zero value = not disabled

	if wasDisabled {
		k.logger.Info("Blocklist re-enabled (temporary disable canceled)")
	}
}

// EnablePolicies immediately re-enables policies (cancels temporary disable)
func (k *KillSwitchManager) EnablePolicies() {
	k.mu.Lock()
	defer k.mu.Unlock()

	wasDisabled := time.Now().Before(k.policiesDisabledUntil)
	k.policiesDisabledUntil = time.Time{} // Zero value = not disabled

	if wasDisabled {
		k.logger.Info("Policies re-enabled (temporary disable canceled)")
	}
}

// IsBlocklistDisabled returns whether the blocklist is currently disabled
// and the time when it will auto-re-enable (if applicable)
func (k *KillSwitchManager) IsBlocklistDisabled() (disabled bool, until time.Time) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	now := time.Now()
	if now.Before(k.blocklistDisabledUntil) {
		return true, k.blocklistDisabledUntil
	}
	return false, time.Time{}
}

// IsPoliciesDisabled returns whether policies are currently disabled
// and the time when they will auto-re-enable (if applicable)
func (k *KillSwitchManager) IsPoliciesDisabled() (disabled bool, until time.Time) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	now := time.Now()
	if now.Before(k.policiesDisabledUntil) {
		return true, k.policiesDisabledUntil
	}
	return false, time.Time{}
}

// GetStatus returns the current status of both kill-switches
func (k *KillSwitchManager) GetStatus() (blocklistDisabled bool, blocklistUntil time.Time, policiesDisabled bool, policiesUntil time.Time) {
	blocklistDisabled, blocklistUntil = k.IsBlocklistDisabled()
	policiesDisabled, policiesUntil = k.IsPoliciesDisabled()
	return
}
