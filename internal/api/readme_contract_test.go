package api

import (
	"os"
	"strings"
	"testing"
)

func TestREADMEListsParameterizedDeleteRoutes(t *testing.T) {
	content, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(content)
	for _, route := range []string{
		"`DELETE /api/v1/api_keys/:id`",
		"`DELETE /api/v1/oauth/apps/:id`",
		"`DELETE /api/v1/users/current/share_tokens/:id`",
		"`DELETE /api/v1/users/current/custom_rules/:rule_id`",
		"`DELETE /api/v1/users/current/leaderboards/:board/members/:user`",
	} {
		if !strings.Contains(readme, route) {
			t.Fatalf("expected README implemented API list to include %s", route)
		}
	}
}
