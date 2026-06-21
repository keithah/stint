package api

import (
	"testing"

	"github.com/keithah/stint/internal/config"
)

func TestFrontendURLUsesConfiguredWebBaseURL(t *testing.T) {
	server := &Server{Config: config.Config{WebBaseURL: "https://app.example.test/base"}}

	got := server.frontendURL("/dashboard")

	if got != "https://app.example.test/dashboard" {
		t.Fatalf("expected configured web base URL, got %q", got)
	}
}

func TestFrontendURLFallsBackToPathForInvalidConfiguredWebBaseURL(t *testing.T) {
	server := &Server{Config: config.Config{WebBaseURL: ":// bad"}}

	got := server.frontendURL("/login")

	if got != "/login" {
		t.Fatalf("expected path fallback for invalid web base URL, got %q", got)
	}
}
