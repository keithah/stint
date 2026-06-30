package stintcli

import (
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stintcli-test-home-*")
	if err != nil {
		panic(err)
	}
	oldHome, hadHome := os.LookupEnv("WAKATIME_HOME")
	oldUserHome, hadUserHome := os.LookupEnv("HOME")
	if err := os.Setenv("WAKATIME_HOME", dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	if hadHome {
		_ = os.Setenv("WAKATIME_HOME", oldHome)
	} else {
		_ = os.Unsetenv("WAKATIME_HOME")
	}
	if hadUserHome {
		_ = os.Setenv("HOME", oldUserHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
