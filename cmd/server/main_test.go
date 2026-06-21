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
	dbIndex := strings.Index(text, "db.Open(ctx, cfg.DatabaseURL)")
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
