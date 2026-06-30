package stintcli

import (
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParseCommonEmptyPrimaryFlagsFallBackToDeprecatedAliasesLikeWakaTime(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "alias.log")
	opts, err := parseCommon([]string{
		"--key=",
		"--api-key", "waka_alias",
		"--api-url=",
		"--apiurl", "http://alias.example/api/v1",
		"--log-file=",
		"--logfile", logFile,
		"--hide-file-names=",
		"--hidefilenames", "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_alias" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
	if opts.APIURL != "http://alias.example/api/v1" {
		t.Fatalf("APIURL = %q", opts.APIURL)
	}
	if opts.LogFile != logFile {
		t.Fatalf("LogFile = %q", opts.LogFile)
	}
	if opts.HideFileNames != "true" {
		t.Fatalf("HideFileNames = %q", opts.HideFileNames)
	}
}

func TestParseCommonUsesWakaTimeAPIKeyEnvFallback(t *testing.T) {
	t.Setenv("WAKATIME_API_KEY", "waka_env")
	config := filepath.Join(t.TempDir(), "missing.cfg")
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_env" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}

	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_config")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err = parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_config" {
		t.Fatalf("config api_key should take precedence over WAKATIME_API_KEY, got %q", opts.APIKey)
	}
}

func TestWakaTimeHomeResourcePathsMatchUpstream(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	if DefaultWakaTimeConfigPath() != filepath.Join(dir, ".wakatime.cfg") {
		t.Fatalf("config path = %q", DefaultWakaTimeConfigPath())
	}
	if DefaultQueuePath() != filepath.Join(dir, "offline_heartbeats.bdb") {
		t.Fatalf("queue path = %q", DefaultQueuePath())
	}
	if DefaultLegacyQueuePath() != filepath.Join(dir, ".wakatime.bdb") {
		t.Fatalf("legacy queue path = %q", DefaultLegacyQueuePath())
	}
	if DefaultLogFilePath() != filepath.Join(dir, "wakatime.log") {
		t.Fatalf("log path = %q", DefaultLogFilePath())
	}
	if DefaultInternalConfigPath() != filepath.Join(dir, "wakatime-internal.cfg") {
		t.Fatalf("internal config path = %q", DefaultInternalConfigPath())
	}
}

func TestParseCommonUsesWakaTimeCompatibleDefaults(t *testing.T) {
	opts, err := parseCommon([]string{"--config", filepath.Join(t.TempDir(), "missing.cfg")})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != defaultTimeoutSeconds {
		t.Fatalf("timeout = %d", opts.Timeout)
	}
	if opts.PrintOffline != defaultPrintOfflineMax {
		t.Fatalf("print offline default = %d", opts.PrintOffline)
	}
	if opts.SyncOffline != defaultQueueMaxSync {
		t.Fatalf("sync offline default = %d", opts.SyncOffline)
	}
}

func TestParseCommonPreservesExplicitZeroTimeoutLikeWakaTime(t *testing.T) {
	opts, err := parseCommon([]string{
		"--timeout", "0",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != 0 {
		t.Fatalf("timeout = %d", opts.Timeout)
	}
}

func TestParseCommonDisablesNegativeHeartbeatRateLimitLikeWakaTime(t *testing.T) {
	opts, err := parseCommon([]string{
		"--heartbeat-rate-limit-seconds", "-1",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HeartbeatRateLimit != 0 {
		t.Fatalf("negative heartbeat rate limit should disable rate limiting, got %d", opts.HeartbeatRateLimit)
	}
}

func TestParseCommonRejectsInvalidWakaTimeCategoryAndEntityType(t *testing.T) {
	if _, err := parseCommon([]string{"--category", "invalid"}); err == nil || !strings.Contains(err.Error(), `invalid category "invalid"`) {
		t.Fatalf("expected invalid category error, got %v", err)
	}
	if _, err := parseCommon([]string{"--entity-type", "invalid"}); err == nil || !strings.Contains(err.Error(), `invalid entity type "invalid"`) {
		t.Fatalf("expected invalid entity type error, got %v", err)
	}
	if opts, err := parseCommon([]string{"--entity-type", "event", "--category", "writing docs"}); err != nil || opts.EntityType != "event" || opts.Category != "writing docs" {
		t.Fatalf("expected valid event/category, opts=%#v err=%v", opts, err)
	}
}

func TestParseCommonRejectsInvalidStatusBarOptionsLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	for key, value := range map[string]string{
		"status_bar_enabled":         "invalid",
		"status_bar_coding_activity": "invalid",
		"status_bar_show_categories": "invalid",
		"status_bar_hide_minutes":    "invalid",
		"status_bar_max_categories":  "-1",
	} {
		config := filepath.Join(dir, key+".cfg")
		cfg := Config{Sections: map[string]map[string]string{}}
		cfg.Set("settings", key, value)
		cfg.Set("settings", "api_key", "waka_test")
		if err := cfg.Write(config); err != nil {
			t.Fatal(err)
		}
		_, err := parseCommon([]string{"--config", config})
		if err == nil {
			t.Fatalf("expected invalid status bar error for %s", key)
		}
	}
	if _, err := parseCommon([]string{"--today-hide-categories", "invalid"}); err == nil {
		t.Fatalf("expected invalid today-hide-categories error")
	}
	if _, err := parseCommon([]string{"--output", "invalid"}); err == nil {
		t.Fatalf("expected invalid output error")
	}
}
