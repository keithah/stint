package api

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/keithah/stint/internal/config"
)

func TestSpecEndpointIndexMatchesRegisteredRoutes(t *testing.T) {
	content, err := os.ReadFile("../../docs/SPEC.md")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	router := NewRouter(config.Config{}, nil)
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		if route.Method == "echo_route_not_found" || routeExcludedFromSpecEndpointIndex(route.Path) {
			continue
		}
		registered[route.Method+" "+route.Path] = true
	}

	listed := map[string]bool{}
	for _, endpoint := range specEndpointIndex(string(content)) {
		key := endpoint.method + " " + endpoint.path
		listed[key] = true
		if !registered[key] {
			t.Fatalf("spec endpoint index lists unregistered route %s", key)
		}
	}
	for key := range registered {
		if !listed[key] {
			t.Fatalf("registered route is missing from spec endpoint index: %s", key)
		}
	}
}

type specEndpoint struct {
	method string
	path   string
}

func specEndpointIndex(spec string) []specEndpoint {
	pattern := regexp.MustCompile(`(?m)^\|\s*(GET|POST|PUT|DELETE)\s*\|\s*\x60([^\x60]+)\x60\s*\|`)
	matches := pattern.FindAllStringSubmatch(spec, -1)
	endpoints := make([]specEndpoint, 0, len(matches))
	for _, match := range matches {
		endpoints = append(endpoints, specEndpoint{method: match[1], path: match[2]})
	}
	return endpoints
}

func routeExcludedFromSpecEndpointIndex(path string) bool {
	if strings.HasPrefix(path, "/api/v1/dev/") {
		return true
	}
	switch path {
	case "/healthz", "/healthz/ingestion", "/auth/github/login", "/auth/github/callback", "/auth/logout":
		return true
	default:
		return false
	}
}
