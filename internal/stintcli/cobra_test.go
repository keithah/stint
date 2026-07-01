package stintcli

import (
	"bytes"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCobraManagedCommandsShowHelp(t *testing.T) {
	for _, args := range [][]string{
		{"setup", "--help"},
		{"collect", "--help"},
		{"cli", "install", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Usage:") {
				t.Fatalf("expected help output, got %q", out.String())
			}
		})
	}
}
