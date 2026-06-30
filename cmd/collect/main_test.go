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

func TestCollectScanConcurrencyBoundsGoroutineCreation(t *testing.T) {
	sourceBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "scanOnce")
	acquire := strings.Index(body, "sem <- struct{}{}")
	launch := strings.Index(body, "go func()")
	if acquire < 0 || launch < 0 {
		t.Fatal("scanOnce should use a semaphore around scan goroutines")
	}
	if acquire > launch {
		t.Fatal("scanOnce must acquire the semaphore before launching goroutines")
	}
}

func functionSource(source, name string) string {
	start := strings.Index(source, "func "+name)
	if start < 0 {
		return ""
	}
	next := strings.Index(source[start+1:], "\nfunc ")
	if next < 0 {
		return source[start:]
	}
	return source[start : start+1+next]
}
