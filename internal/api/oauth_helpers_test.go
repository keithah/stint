package api

import (
	"strings"
	"testing"
)

func TestOAuthErrorHTMLEscapesMessage(t *testing.T) {
	got := oauthErrorHTML(`<script>alert("x")</script>`)
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected OAuth error HTML to escape script tags, got %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped script tag in OAuth error HTML, got %s", got)
	}
}

func TestSplitScopesTrimsDeduplicatesAndAcceptsCommas(t *testing.T) {
	got := splitScopes(" read_stats,read_summaries read_stats  ")
	want := []string{"read_stats", "read_summaries"}
	if len(got) != len(want) {
		t.Fatalf("expected scopes %#v, got %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected scopes %#v, got %#v", want, got)
		}
	}
}
