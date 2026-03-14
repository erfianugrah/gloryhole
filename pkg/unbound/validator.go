package unbound

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Validate writes the config to a temp file and runs unbound-checkconf.
// Returns nil if valid, or an error with the checkconf output.
func Validate(cfg *UnboundServerConfig, checkconfBin string) error {
	tmp, err := os.CreateTemp("", "unbound-*.conf")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if writeErr := WriteConfig(cfg, tmp.Name()); writeErr != nil {
		return fmt.Errorf("write temp config: %w", writeErr)
	}

	out, err := exec.Command(checkconfBin, tmp.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unbound-checkconf: %s", strings.TrimSpace(string(out)))
	}

	return nil
}
