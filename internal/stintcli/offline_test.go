package stintcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	_ "modernc.org/sqlite"
)

func TestConfigParseErrorFallsBackToOfflineQueueLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	entity := filepath.Join(dir, "main.go")
	if err := os.WriteFile(entity, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(dir, "offline.bdb")
	err := Run([]string{
		"--config", dir,
		"--entity", entity,
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected config parse error")
	}
	count, countErr := CountQueue(queue)
	if countErr != nil {
		t.Fatal(countErr)
	}
	if count != 1 {
		t.Fatalf("queued fallback heartbeats = %d, want 1", count)
	}
}

func TestParseCommonUsesSettingsOfflineUnlessDisableFlagSet(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "offline", "false")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.DisableOffline {
		t.Fatalf("expected settings.offline=false to disable offline queue")
	}

	opts, err = parseCommon([]string{"--config", config, "--disable-offline=false"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.DisableOffline {
		t.Fatalf("explicit --disable-offline=false should take precedence over settings.offline=false")
	}
}

func TestParseCommonUsesProjectSettingsOffline(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "offline", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	projectCfg := Config{Sections: map[string]map[string]string{}}
	projectCfg.Set("settings", "offline", "false")
	if err := projectCfg.Write(filepath.Join(dir, ".wakatime")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{"--config", config, "--entity", file})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.DisableOffline {
		t.Fatalf("expected project settings.offline=false to disable offline queue")
	}
}

func TestHeartbeatActiveBackoffQueuesWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	internalConfig := filepath.Join(dir, "wakatime-internal.cfg")
	if err := WriteConfigValue(internalConfig, "internal", "backoff_retries", "1"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(internalConfig, "internal", "backoff_at", time.Now().Format(wakaTimeDateFormat)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--entity", file,
		"--offline-queue-file", filepath.Join(dir, "offline.bdb"),
		"--internal-config", internalConfig,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("expected no network call during backoff, got %d", calls)
	}
	if strings.TrimSpace(out.String()) != "queued=1" {
		t.Fatalf("expected queued output, got %q", out.String())
	}
}

func TestDisableOfflineFlagTakesPrecedenceOverDeprecatedAlias(t *testing.T) {
	opts, err := parseCommon([]string{
		"--disable-offline=false",
		"--disableoffline=true",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.DisableOffline {
		t.Fatal("deprecated disableoffline alias overrode explicit disable-offline=false")
	}
}

func TestOfflineQueueCountPrintAndSync(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{{Entity: "/tmp/a.go", EntityType: "file", Time: 1}, {Entity: "/tmp/b.go", EntityType: "file", Time: 2}}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %d", len(posted))
	}
	count, err = CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after sync = %d", count)
	}
}

func TestOfflineQueueFileHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	err := Run([]string{
		"--offline-count",
		"--offline-queue-file", "~missing-user/offline_heartbeats.bdb",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed expanding offline-queue-file param") {
		t.Fatalf("expected offline queue expansion error, got %v", err)
	}
}

func TestOfflineSyncFansOutToConfiguredAPIURLs(t *testing.T) {
	dir := t.TempDir()
	queue := filepath.Join(dir, "offline.bdb")
	heartbeats := []Heartbeat{{Entity: "/work/projects/main.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.Header.Get("Authorization")]++
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_url", server.URL+"/api/v1")
	cfg.Set("settings", "api_key", "waka_default")
	cfg.Set("api_urls", `/work/`, server.URL+"/api/v1|waka_fanout")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"offline", "sync", "--config", config, "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if counts[basicAuthHeader("waka_default")] != 1 || counts[basicAuthHeader("waka_fanout")] != 1 {
		t.Fatalf("unexpected fanout counts: %#v", counts)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after sync = %d", count)
	}
}

func TestPrintOfflineDefaultLimitMatchesWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	var heartbeats []Heartbeat
	for i := 0; i < defaultPrintOfflineMax+2; i++ {
		heartbeats = append(heartbeats, Heartbeat{Entity: fmt.Sprintf("/tmp/%02d.go", i), EntityType: "file", Time: float64(i + 1)})
	}
	heartbeats[0].Entity = "/tmp/<main>&.go"
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"offline", "print", "--offline-queue-file", queue, "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var printed []Heartbeat
	if err := json.Unmarshal(out.Bytes(), &printed); err != nil {
		t.Fatal(err)
	}
	if len(printed) != defaultPrintOfflineMax {
		t.Fatalf("printed = %d", len(printed))
	}
	if strings.Contains(out.String(), "\n  ") || !strings.Contains(out.String(), `"/tmp/<main>&.go"`) {
		t.Fatalf("unexpected offline print format: %q", out.String())
	}
}

func TestPrintOfflineExplicitZeroPrintsNoHeartbeatsLikeWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{{Entity: "/tmp/queued.go", EntityType: "file", Time: 1}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"offline", "print", "--print-offline-heartbeats", "0", "--offline-queue-file", queue, "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSendHeartbeatsQueuesExtraHeartbeatsOverWakaTimeSendLimit(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := make([]Heartbeat, 0, defaultHeartbeatLimit+2)
	for i := 0; i < defaultHeartbeatLimit+2; i++ {
		heartbeats = append(heartbeats, Heartbeat{
			Entity:     fmt.Sprintf("/tmp/live-%02d.go", i),
			EntityType: "file",
			Time:       float64(i + 1),
		})
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	err := sendHeartbeats(&bytes.Buffer{}, Options{
		APIKey:             "waka_test",
		APIURL:             server.URL + "/api/v1",
		HeartbeatRateLimit: 0,
		InternalConfigPath: filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		QueuePath:          queue,
		Timeout:            1,
	}, heartbeats, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(posted) != defaultHeartbeatLimit {
		t.Fatalf("posted = %d", len(posted))
	}
	queued, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 || queued[0].Entity != "/tmp/live-10.go" || queued[1].Entity != "/tmp/live-11.go" {
		t.Fatalf("queued extras = %#v", queued)
	}
}

func TestRootEntityOfflineSyncHonorsExplicitSyncLimitLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(dir, "offline.bdb")
	queued := []Heartbeat{
		{Entity: "/tmp/queued-1.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/queued-2.go", EntityType: "file", Time: 2},
	}
	if err := AppendQueue(queue, queued); err != nil {
		t.Fatal(err)
	}

	var calls [][]Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, posted)
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", file,
		"--sync-offline-activity", "1",
		"--sync-ai-disabled",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected live heartbeat plus one offline sync call, got %#v", calls)
	}
	if len(calls[1]) != 1 || calls[1][0].Entity != "/tmp/queued-1.go" {
		t.Fatalf("explicit sync limit should post one queued heartbeat, got %#v", calls[1])
	}
	remaining, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Entity != "/tmp/queued-2.go" {
		t.Fatalf("expected second queued heartbeat to remain, got %#v", remaining)
	}
}

func TestOfflineQueueDeleteDuplicates(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10.5},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 12},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 10.5},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	deleted, err := DeleteQueueDuplicates(queue)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d", deleted)
	}
	got, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("queue length = %d: %#v", len(got), got)
	}
	for _, hb := range got {
		if hb.Entity == "/tmp/a.go" && hb.Time == 10.5 {
			t.Fatalf("duplicate heartbeat was not removed: %#v", got)
		}
	}
}

func TestOfflineSyncDeduplicatesBeforePosting(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10.5},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 12},
	}); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	if posted[0].Time != 10 || posted[1].Time != 12 {
		t.Fatalf("unexpected posted heartbeat times: %#v", posted)
	}
}

func TestOfflineSyncLeavesRequeuedHeartbeatsForNextRun(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	original := make([]Heartbeat, 0, defaultHeartbeatLimit+1)
	for i := 0; i < defaultHeartbeatLimit+1; i++ {
		original = append(original, Heartbeat{
			Entity:     fmt.Sprintf("/tmp/%03d.go", i),
			EntityType: "file",
			Time:       float64(i + 1),
		})
	}
	if err := AppendQueue(queue, original); err != nil {
		t.Fatal(err)
	}

	var calls [][]Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, posted)
		responses := make([]map[string]any, len(posted))
		for i := range posted {
			responses[i] = map[string]any{"status": http.StatusTooManyRequests}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"responses": responses})
	}))
	defer server.Close()

	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("sync posted %d batches, want 2 original queue batches", len(calls))
	}
	if len(calls[0]) != defaultHeartbeatLimit || len(calls[1]) != 1 {
		t.Fatalf("sync should only post the original queue snapshot, got batch sizes %d and %d", len(calls[0]), len(calls[1]))
	}
	remaining, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != len(original) {
		t.Fatalf("remaining queue length = %d, want %d: %#v", len(remaining), len(original), remaining)
	}
}

func TestOfflineSyncRequeuesFailedAndMissingResults(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/ok.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/retry.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/missing.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"responses": []any{
			[]any{map[string]any{"data": posted[0]}, http.StatusCreated},
			[]any{map[string]any{"data": posted[1]}, http.StatusInternalServerError},
		}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Entity != "/tmp/retry.go" || got[1].Entity != "/tmp/missing.go" {
		t.Fatalf("unexpected requeued heartbeats: %#v", got)
	}
}

func TestOfflineSyncRequeuesMissingResultByHeartbeatAssociationLikeWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/c.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"responses": []any{
			[]any{map[string]any{"data": posted[0]}, http.StatusCreated},
			[]any{map[string]any{"data": posted[2]}, http.StatusCreated},
		}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != heartbeats[1].ID() {
		t.Fatalf("unexpected requeued heartbeats: %#v", got)
	}
}

func TestOfflineSyncDropsBadRequestResults(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	hb := Heartbeat{Entity: "/tmp/bad.go", EntityType: "file", Time: 1}
	if err := AppendQueue(queue, []Heartbeat{hb}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"responses": []any{[]any{map[string]any{"data": posted[0]}, http.StatusBadRequest}}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("bad request heartbeat should be dropped, count=%d", count)
	}
}

func TestOfflineSyncZeroLimitSyncsAllQueuedHeartbeats(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/c.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--sync-offline-activity", "0", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != len(heartbeats) {
		t.Fatalf("posted = %d", len(posted))
	}
}

func TestOfflineSyncPostsWakaTimeSizedChunks(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := make([]Heartbeat, 0, 25)
	for i := 0; i < 25; i++ {
		heartbeats = append(heartbeats, Heartbeat{
			Entity:     fmt.Sprintf("/tmp/offline-%02d.go", i),
			EntityType: "file",
			Time:       float64(i + 1),
		})
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		batchSizes = append(batchSizes, len(posted))
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"offline", "sync", "--sync-offline-activity", "25", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "synced=25" {
		t.Fatalf("output = %q", out.String())
	}
	if got := fmt.Sprint(batchSizes); got != "[10 10 5]" {
		t.Fatalf("batch sizes = %s", got)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after sync = %d", count)
	}
}

func TestOfflineSyncNegativeLimitSyncsAllQueuedHeartbeatsLikeWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/c.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--sync-offline-activity", "-1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != len(heartbeats) {
		t.Fatalf("posted = %d", len(posted))
	}
}

func TestOfflineSyncMigratesConfiguredLegacyBoltQueueLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	queue := filepath.Join(dir, "offline_heartbeats.bdb")
	legacyQueue := filepath.Join(dir, ".wakatime.bdb")
	heartbeats := []Heartbeat{{Entity: "/tmp/legacy.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(legacyQueue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{
		"offline", "sync",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--offline-queue-file-legacy", legacyQueue,
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != "/tmp/legacy.go" {
		t.Fatalf("posted = %#v", posted)
	}
	if _, err := os.Stat(legacyQueue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy queue should be removed after sync, stat err=%v", err)
	}
}

func TestOfflineSyncEmptyLegacyQueueFlagFallsBackToDefaultLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	queue := filepath.Join(dir, "offline_heartbeats.bdb")
	legacyQueue := filepath.Join(dir, ".wakatime.bdb")
	heartbeats := []Heartbeat{{Entity: "/tmp/default-legacy.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(legacyQueue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{
		"offline", "sync",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--offline-queue-file-legacy=",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != "/tmp/default-legacy.go" {
		t.Fatalf("posted = %#v", posted)
	}
	if _, err := os.Stat(legacyQueue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy queue should be removed after sync, stat err=%v", err)
	}
}

func TestOfflineQueueUsesWakaTimeBoltBucketAndKeys(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	hb := Heartbeat{Entity: "/tmp/a.go", EntityType: "file", Category: "coding", Project: "stint", Branch: "main", IsWrite: true, Time: 1}
	if err := AppendQueue(queue, []Heartbeat{hb}); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(queue, 0o600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("heartbeats"))
		if b == nil {
			t.Fatalf("missing heartbeats bucket")
		}
		var got Heartbeat
		if err := json.Unmarshal(b.Get([]byte(hb.ID())), &got); err != nil {
			t.Fatal(err)
		}
		if got.Entity != hb.Entity || got.Project != hb.Project {
			t.Fatalf("unexpected queued heartbeat: %#v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineQueueResetsCorruptBoltDBLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	queue := filepath.Join(dir, "offline.bdb")
	if err := os.WriteFile(queue, []byte("not a bolt database"), 0o600); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count = %d", count)
	}
	backups, err := filepath.Glob(queue + ".corrupt.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("corrupt backups = %v", backups)
	}
	hb := Heartbeat{Entity: "/tmp/recovered.go", EntityType: "file", Time: 1}
	if err := AppendQueue(queue, []Heartbeat{hb}); err != nil {
		t.Fatal(err)
	}
	read, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || read[0].Entity != hb.Entity {
		t.Fatalf("read = %#v", read)
	}
}

func TestOfflineQueueLegacyJSONLStillWorks(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.jsonl")
	heartbeats := []Heartbeat{{Entity: "/tmp/a.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	read, err := ReadQueue(queue, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || read[0].Entity != "/tmp/a.go" {
		t.Fatalf("read = %#v", read)
	}
	if err := RemoveQueuePrefix(queue, 1); err != nil {
		t.Fatal(err)
	}
	count, err = CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after remove = %d", count)
	}
}

func TestRootSyncAIActivityTakesPrecedenceOverOfflineCountLikeWakaTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "27")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-priority.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","type":"session_meta","payload":{"id":"codex-priority","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-27T12:01:00Z","payload":{"message":"Update the main file","filePath":"main.go"}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{{Entity: "/tmp/offline.go", EntityType: "file", Time: 1}}); err != nil {
		t.Fatal(err)
	}

	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{
		"--sync-ai-activity",
		"--offline-count",
		"--sync-ai-after", "1780000000",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) == 0 {
		t.Fatalf("sync-ai-activity should take precedence over offline-count; output=%q", out.String())
	}
	if strings.TrimSpace(out.String()) == "1" {
		t.Fatalf("offline-count handled mixed command before sync-ai-activity")
	}
}
