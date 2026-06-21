package api

import (
	"os"
	"strings"
	"testing"
)

func TestTokenOnlyShareRoutesAreRegistered(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("could not read router.go: %v", err)
	}
	text := string(source)
	for _, needle := range []string{
		`api.GET("/share/:token/stats", server.publicShareStatsByToken`,
		`api.GET("/share/:token/summaries", server.publicShareSummariesByToken`,
		`add("/api/v1/share/{token}/stats"`,
		`add("/api/v1/share/{token}/summaries"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected token-only share support to include %q", needle)
		}
	}
}
