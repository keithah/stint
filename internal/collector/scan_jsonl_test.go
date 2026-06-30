package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanJSONLIncrementalSkipsOversizedCompleteLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxJSONLLineBytes+1)+"\n{\"ok\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewState()
	report := &ScanReport{}
	var lines [][]byte
	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		lines = append(lines, append([]byte(nil), line...))
	})

	if report.Errors != 1 {
		t.Fatalf("Errors = %d, want 1 for oversized line", report.Errors)
	}
	if len(lines) != 1 || string(lines[0]) != "{\"ok\":true}\n" {
		t.Fatalf("lines = %q, want only the normal complete line", lines)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if offset, _ := state.ByteOffset(path, info.Size()); offset == 0 {
		t.Fatal("scanner should commit past the oversized line instead of retrying it forever")
	}
}
