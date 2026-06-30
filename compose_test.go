package stint_test

import (
	"os"
	"strings"
	"testing"
)

func TestComposeUsesHealthyDependenciesAndRestartPolicies(t *testing.T) {
	sourceBytes, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	for _, service := range []string{"postgres:", "redis:", "api:", "worker:", "web:", "collector:"} {
		section := composeServiceSection(source, service)
		if !strings.Contains(section, "restart:") {
			t.Fatalf("%s should declare a restart policy", service)
		}
	}
	redis := composeServiceSection(source, "redis:")
	if !strings.Contains(redis, "healthcheck:") || !strings.Contains(redis, "redis-cli ping") {
		t.Fatal("redis should expose a real healthcheck")
	}
	for _, service := range []string{"api:", "worker:"} {
		section := composeServiceSection(source, service)
		if !strings.Contains(section, "redis:") || !strings.Contains(section, "condition: service_healthy") {
			t.Fatalf("%s should wait for redis to be healthy", service)
		}
	}
	worker := composeServiceSection(source, "worker:")
	if !strings.Contains(worker, "healthcheck:") {
		t.Fatal("worker should override the api image healthcheck with a worker-safe probe")
	}
}

func composeServiceSection(source, service string) string {
	start := strings.Index(source, "\n  "+service)
	if start == -1 {
		start = strings.Index(source, "  "+service)
	}
	if start == -1 {
		return ""
	}
	rest := source[start:]
	next := strings.Index(rest[1:], "\n\n  ")
	if next == -1 {
		return rest
	}
	return rest[:1+next]
}
