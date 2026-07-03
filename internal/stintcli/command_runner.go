package stintcli

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func runCommandDiscardingOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // Callers pass command paths selected from fixed registries.
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("%s: %w", message, err)
		}
		return err
	}
	return nil
}
