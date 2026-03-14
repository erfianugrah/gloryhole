// Package unbound provides process supervision for the Unbound DNS resolver.
// When enabled, Glory-Hole starts Unbound as a child process on localhost:5353,
// forwarding allowed queries to it for recursive resolution and DNSSEC validation.
package unbound

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	mdns "github.com/miekg/dns"
)

// State represents the supervisor's current lifecycle state.
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateDegraded State = "degraded" // started but health checks failing
	StateFailed   State = "failed"   // gave up restarting
)

// Supervisor manages the Unbound child process.
type Supervisor struct {
	cfg    *config.UnboundConfig
	logger *logging.Logger

	mu           sync.Mutex
	state        State
	lastErr      error
	cmd          *exec.Cmd
	cancelHealth context.CancelFunc
	cancelProc   context.CancelFunc
	serverConfig *UnboundServerConfig // Current in-memory config

	// Restart tracking
	restartCount    int
	restartWindowAt time.Time

	// Paths (auto-detected or configured)
	binaryPath   string
	controlBin   string
	checkconfBin string
	anchorBin    string
	listenAddr   string
}

// NewSupervisor creates a new Unbound process supervisor.
func NewSupervisor(cfg *config.UnboundConfig, logger *logging.Logger) *Supervisor {
	return &Supervisor{
		cfg:        cfg,
		logger:     logger,
		state:      StateStopped,
		listenAddr: fmt.Sprintf("127.0.0.1:%d", cfg.ListenPort),
	}
}

// State returns the current supervisor state and last error.
func (s *Supervisor) Status() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.lastErr
}

// ListenAddr returns the address Unbound is listening on.
func (s *Supervisor) ListenAddr() string {
	return s.listenAddr
}

// ServerConfig returns the current in-memory Unbound server config.
func (s *Supervisor) ServerConfig() *UnboundServerConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serverConfig
}

// SetServerConfig updates the in-memory config (called after API writes).
func (s *Supervisor) SetServerConfig(cfg *UnboundServerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverConfig = cfg
}

// CheckconfBin returns the path to unbound-checkconf.
func (s *Supervisor) CheckconfBin() string {
	return s.checkconfBin
}

// Start begins the Unbound process and waits for readiness.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == StateRunning || s.state == StateStarting {
		s.mu.Unlock()
		return nil
	}
	s.state = StateStarting
	s.mu.Unlock()

	// Auto-detect binary paths
	if err := s.detectBinaries(); err != nil {
		s.setState(StateFailed, err)
		return fmt.Errorf("unbound binaries not found: %w", err)
	}

	// Initialize default server config if not already set
	s.mu.Lock()
	if s.serverConfig == nil {
		s.serverConfig = DefaultServerConfig(s.cfg.ListenPort, s.cfg.ControlSocket)
	}
	s.mu.Unlock()

	// Bootstrap DNSSEC root trust anchor (idempotent)
	s.bootstrapAnchor()

	// Check for orphaned process on our port
	if err := s.checkPort(); err != nil {
		s.setState(StateFailed, err)
		return err
	}

	// Start the process
	if err := s.startProcess(ctx); err != nil {
		s.setState(StateFailed, err)
		return fmt.Errorf("failed to start unbound: %w", err)
	}

	// Wait for readiness
	if err := s.waitReady(ctx, 30*time.Second); err != nil {
		s.stopProcess()
		s.setState(StateFailed, err)
		return fmt.Errorf("unbound readiness timeout: %w", err)
	}

	s.setState(StateRunning, nil)
	s.logger.Info("Unbound resolver started",
		"addr", s.listenAddr,
		"binary", s.binaryPath,
	)

	// Start health monitor
	healthCtx, healthCancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancelHealth = healthCancel
	s.mu.Unlock()
	go s.healthLoop(healthCtx)

	return nil
}

// Stop gracefully shuts down the Unbound process.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if s.cancelHealth != nil {
		s.cancelHealth()
	}
	if s.cancelProc != nil {
		s.cancelProc()
	}
	s.mu.Unlock()

	s.stopProcess()
	s.setState(StateStopped, nil)
	s.logger.Info("Unbound resolver stopped")
	return nil
}

// Reload sends a reload command to Unbound via unbound-control.
func (s *Supervisor) Reload() error {
	return s.runControl("reload")
}

// FlushCache flushes the entire Unbound cache.
func (s *Supervisor) FlushCache() error {
	return s.runControl("flush_zone", ".")
}

// --- internal ---

func (s *Supervisor) setState(state State, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.lastErr = err
}

func (s *Supervisor) detectBinaries() error {
	var err error

	if s.cfg.BinaryPath != "" {
		s.binaryPath = s.cfg.BinaryPath
	} else {
		s.binaryPath, err = findBinary("unbound")
		if err != nil {
			return fmt.Errorf("unbound: %w", err)
		}
	}

	s.controlBin, err = findBinary("unbound-control")
	if err != nil {
		return fmt.Errorf("unbound-control: %w", err)
	}

	s.checkconfBin, err = findBinary("unbound-checkconf")
	if err != nil {
		return fmt.Errorf("unbound-checkconf: %w", err)
	}

	s.anchorBin, _ = findBinary("unbound-anchor") // optional

	s.logger.Info("Unbound binaries detected",
		"unbound", s.binaryPath,
		"control", s.controlBin,
		"checkconf", s.checkconfBin,
	)

	return nil
}

func findBinary(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	// Common fallback paths
	for _, dir := range []string{"/usr/local/bin", "/usr/local/sbin", "/usr/sbin", "/usr/bin"} {
		path := dir + "/" + name
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("%s not found in PATH or common locations", name)
}

func (s *Supervisor) bootstrapAnchor() {
	if s.anchorBin == "" {
		return
	}

	out, err := exec.Command(s.anchorBin, "-a", "/etc/unbound/root.key").CombinedOutput()
	if err != nil {
		// unbound-anchor returns exit code 1 when the anchor was updated (not an error)
		s.logger.Debug("unbound-anchor completed", "output", string(out), "exit", err)
	}
}

func (s *Supervisor) checkPort() error {
	conn, err := net.DialTimeout("tcp", s.listenAddr, 500*time.Millisecond)
	if err != nil {
		return nil // Port is free
	}
	conn.Close()
	return fmt.Errorf("port %s already in use by another process", s.listenAddr)
}

func (s *Supervisor) startProcess(ctx context.Context) error {
	procCtx, procCancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(procCtx, s.binaryPath, "-d", "-c", s.cfg.ConfigPath)

	// Capture stderr for logging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		procCancel()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		procCancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		procCancel()
		return fmt.Errorf("exec: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.cancelProc = procCancel
	s.mu.Unlock()

	// Forward Unbound's output to our logger
	go s.forwardLogs("[unbound] ", stderr)
	go s.forwardLogs("[unbound] ", stdout)

	// Monitor process exit in background
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		state := s.state
		s.mu.Unlock()

		if state == StateStopped {
			return // Expected shutdown
		}

		s.logger.Error("Unbound process exited unexpectedly", "error", err)
		s.handleCrash(ctx)
	}()

	return nil
}

func (s *Supervisor) stopProcess() {
	s.mu.Lock()
	cmd := s.cmd
	cancel := s.cancelProc
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	// Cancel the context (sends SIGKILL after context deadline)
	if cancel != nil {
		cancel()
	}

	// Wait briefly for clean exit
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.logger.Warn("Unbound did not exit cleanly, killing")
		_ = cmd.Process.Kill()
	}

	s.mu.Lock()
	s.cmd = nil
	s.mu.Unlock()
}

func (s *Supervisor) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out after %v waiting for unbound on %s", timeout, s.listenAddr)
		case <-ticker.C:
			if s.probe() {
				return nil
			}
		}
	}
}

func (s *Supervisor) probe() bool {
	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	m := new(mdns.Msg)
	m.SetQuestion(".", mdns.TypeNS)

	_, _, err := c.Exchange(m, s.listenAddr)
	return err == nil
}

func (s *Supervisor) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	consecutiveFailures := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.probe() {
				if consecutiveFailures > 0 {
					s.logger.Info("Unbound health check recovered")
					s.setState(StateRunning, nil)
				}
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				s.logger.Warn("Unbound health check failed",
					"consecutive_failures", consecutiveFailures)

				if consecutiveFailures >= 3 {
					s.setState(StateDegraded, fmt.Errorf("health check failed %d times", consecutiveFailures))
					s.handleCrash(ctx)
					consecutiveFailures = 0
				}
			}
		}
	}
}

func (s *Supervisor) handleCrash(ctx context.Context) {
	s.mu.Lock()
	now := time.Now()

	// Reset restart counter if outside the window
	if now.Sub(s.restartWindowAt) > 60*time.Second {
		s.restartCount = 0
		s.restartWindowAt = now
	}

	s.restartCount++
	count := s.restartCount
	s.mu.Unlock()

	if count > 5 {
		s.logger.Error("Unbound failed too many times, entering failed state",
			"restarts_in_window", count)
		s.setState(StateFailed, fmt.Errorf("exceeded restart limit: %d restarts in 60s", count))
		return
	}

	// Exponential backoff: 1s, 2s, 4s, 8s, 16s (max 30s)
	backoff := time.Duration(1<<uint(count-1)) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}

	s.logger.Info("Restarting Unbound",
		"attempt", count,
		"backoff", backoff,
	)

	select {
	case <-ctx.Done():
		return
	case <-time.After(backoff):
	}

	s.stopProcess()

	if err := s.startProcess(ctx); err != nil {
		s.logger.Error("Failed to restart Unbound", "error", err)
		return
	}

	if err := s.waitReady(ctx, 10*time.Second); err != nil {
		s.logger.Error("Unbound not ready after restart", "error", err)
		s.stopProcess()
		return
	}

	s.setState(StateRunning, nil)
	s.logger.Info("Unbound restarted successfully")
}

func (s *Supervisor) runControl(args ...string) error {
	cmdArgs := append([]string{"-c", s.cfg.ConfigPath, "-s", s.cfg.ControlSocket}, args...)
	out, err := exec.Command(s.controlBin, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unbound-control %v: %w: %s", args, err, out)
	}
	return nil
}

func (s *Supervisor) forwardLogs(prefix string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s.logger.Debug(prefix + scanner.Text())
	}
}
