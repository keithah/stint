package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainValidatesConfigBeforeOpeningDatabase(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	validateIndex := strings.Index(text, "cfg.Validate()")
	dbIndex := strings.Index(text, "db.OpenWithPoolConfig(ctx, cfg.DatabaseURL")
	if validateIndex < 0 {
		t.Fatal("expected main to validate config")
	}
	if dbIndex < 0 {
		t.Fatal("expected main to open database")
	}
	if validateIndex > dbIndex {
		t.Fatal("expected config validation before opening database")
	}
}

func TestMainUsesSignalContextAndHTTPServerTimeouts(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, needle := range []string{
		"signal.NotifyContext",
		"server := router.Server",
		"server.ReadHeaderTimeout =",
		"server.ReadTimeout =",
		"server.WriteTimeout =",
		"server.IdleTimeout =",
		"router.StartServer(server)",
		"server.Shutdown(shutdownCtx)",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected main.go to include %s", needle)
		}
	}
}
