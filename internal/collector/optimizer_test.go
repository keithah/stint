package collector

import (
	"os"
	"strings"
	"testing"
)

func TestJSONSessionScannersStreamDecodeFiles(t *testing.T) {
	for _, path := range []string{"amp.go", "crush.go", "gemini.go"} {
		sourceBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(sourceBytes)
		if strings.Contains(source, "os.ReadFile(path)") {
			t.Fatalf("%s should stream decode session JSON instead of reading the whole file", path)
		}
		if !strings.Contains(source, "decodeJSONFile(path,") {
			t.Fatalf("%s should use decodeJSONFile for size-capped stream decoding", path)
		}
	}
}
