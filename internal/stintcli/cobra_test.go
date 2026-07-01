package stintcli

import (
	"bytes"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCobraManagedCommandsShowHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"setup", "--help"}, []string{"Usage:", "--server", "--key", "--stint-config", "--wakatime-config"}},
		{[]string{"collect", "--help"}, []string{"Usage:", "--api-url", "--agent", "--watch", "--print-config"}},
		{[]string{"cli", "install", "--help"}, []string{"Usage:"}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out bytes.Buffer
			if err := Run(tc.args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("expected help output to contain %q, got %q", want, out.String())
				}
			}
		})
	}
}
