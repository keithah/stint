package stint_test

import (
	"os"
	"strings"
	"testing"
)

func TestProductionDockerfileDoesNotDownloadTestOnlyModuleGraph(t *testing.T) {
	sourceBytes, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	if strings.Contains(source, "go mod download") {
		t.Fatal("production Docker build should not run go mod download because it pulls test-only dependencies")
	}
	if !strings.Contains(source, "go build -o /out/stint ./cmd/server") {
		t.Fatal("production Docker build should still build the server binary")
	}
}

func TestDockerfileHealthchecksMatchRuntimeTargets(t *testing.T) {
	sourceBytes, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	if !strings.Contains(source, "HEALTHCHECK") || !strings.Contains(source, "/healthz") {
		t.Fatal("api image should publish an HTTP /healthz Dockerfile healthcheck")
	}
	collector := source[strings.Index(source, "FROM api AS collector"):]
	if !strings.Contains(collector, "HEALTHCHECK NONE") {
		t.Fatal("collector target should disable the inherited api HTTP healthcheck")
	}
}
