package stintcli

import (
	"os"
	"strings"
	"testing"
)

func TestAITranscriptParsingHasFileAndWalkBounds(t *testing.T) {
	for _, path := range []string{"ai_sources.go", "ai_continue.go", "ai_kiro.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		if !strings.Contains(text, "maxAITranscriptFiles") {
			t.Fatalf("%s should cap transcript file walks", path)
		}
		if !strings.Contains(text, "maxAITranscriptFileBytes") {
			t.Fatalf("%s should cap whole-file transcript reads", path)
		}
	}
	source, err := os.ReadFile("ai_transcript_parse.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "maxAITranscriptFileBytes") || !strings.Contains(text, "readAITranscriptFile") {
		t.Fatal("ai_transcript_parse.go should route whole-file transcript reads through the bounded helper")
	}
}
