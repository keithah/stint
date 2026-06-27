package main

import (
	"os"
	"strings"
	"testing"
)

func TestCollectUploadUsesOverallDeadline(t *testing.T) {
	sourceBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	source := string(sourceBytes)
	if !strings.Contains(source, "context.WithTimeout(context.Background(), collectUploadTimeout)") {
		t.Fatal("collect upload should use an overall deadline")
	}
	if !strings.Contains(source, "defer cancelUpload()") {
		t.Fatal("collect upload deadline context should be canceled")
	}
}
