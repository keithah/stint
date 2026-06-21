package jobs

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/services"
)

func TestStatsRecomputeTaskRoundTripsPayload(t *testing.T) {
	userID := uuid.New()
	task, err := NewStatsRecomputeTask(userID, []string{"last_7_days", "last_30_days"})
	if err != nil {
		t.Fatalf("NewStatsRecomputeTask returned error: %v", err)
	}
	if task.Type() != TypeStatsRecompute {
		t.Fatalf("expected task type %q, got %q", TypeStatsRecompute, task.Type())
	}

	payload, err := ParseStatsRecomputeTask(task)
	if err != nil {
		t.Fatalf("ParseStatsRecomputeTask returned error: %v", err)
	}
	if payload.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, payload.UserID)
	}
	if len(payload.Ranges) != 2 || payload.Ranges[1] != "last_30_days" {
		t.Fatalf("unexpected ranges: %#v", payload.Ranges)
	}
}

func TestDefaultStatsRangesIncludesSupportedDashboardRanges(t *testing.T) {
	got := DefaultStatsRanges()
	want := []string{"last_7_days", "last_30_days", "last_6_months", "last_year", "all_time"}
	if len(got) != len(want) {
		t.Fatalf("expected %d ranges, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected range %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestGoalsEvaluateTaskRoundTripsPayload(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	task, err := NewGoalsEvaluateTask(now)
	if err != nil {
		t.Fatalf("NewGoalsEvaluateTask returned error: %v", err)
	}
	if task.Type() != TypeGoalsEvaluate {
		t.Fatalf("expected task type %q, got %q", TypeGoalsEvaluate, task.Type())
	}

	payload, err := ParseGoalsEvaluateTask(task)
	if err != nil {
		t.Fatalf("ParseGoalsEvaluateTask returned error: %v", err)
	}
	if got := GoalsEvaluateNow(payload); !got.Equal(now) {
		t.Fatalf("expected now %s, got %s", now, got)
	}
	if payload.Scheduled {
		t.Fatal("manual goals evaluation task should not be marked scheduled")
	}
}

func TestScheduledGoalsEvaluateTaskMarksPayloadScheduled(t *testing.T) {
	task, err := NewScheduledGoalsEvaluateTask()
	if err != nil {
		t.Fatalf("NewScheduledGoalsEvaluateTask returned error: %v", err)
	}
	if task.Type() != TypeGoalsEvaluate {
		t.Fatalf("expected task type %q, got %q", TypeGoalsEvaluate, task.Type())
	}

	payload, err := ParseGoalsEvaluateTask(task)
	if err != nil {
		t.Fatalf("ParseGoalsEvaluateTask returned error: %v", err)
	}
	if !payload.Scheduled {
		t.Fatal("expected scheduled goals task to mark payload as scheduled")
	}
	if payload.NowUnix != 0 {
		t.Fatalf("scheduled goals task should use worker execution time, got now_unix=%d", payload.NowUnix)
	}
}

func TestDataDumpProcessTaskRoundTripsPayload(t *testing.T) {
	userID := uuid.New()
	dumpID := uuid.New()
	task, err := NewDataDumpProcessTask(userID, dumpID)
	if err != nil {
		t.Fatalf("NewDataDumpProcessTask returned error: %v", err)
	}
	if task.Type() != TypeDataDumpProcess {
		t.Fatalf("expected task type %q, got %q", TypeDataDumpProcess, task.Type())
	}

	payload, err := ParseDataDumpProcessTask(task)
	if err != nil {
		t.Fatalf("ParseDataDumpProcessTask returned error: %v", err)
	}
	if payload.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, payload.UserID)
	}
	if payload.DumpID != dumpID {
		t.Fatalf("expected dump id %s, got %s", dumpID, payload.DumpID)
	}
}

func TestCustomRulesApplyTaskRoundTripsPayload(t *testing.T) {
	userID := uuid.New()
	task, err := NewCustomRulesApplyTask(userID)
	if err != nil {
		t.Fatalf("NewCustomRulesApplyTask returned error: %v", err)
	}
	if task.Type() != TypeCustomRulesApply {
		t.Fatalf("expected task type %q, got %q", TypeCustomRulesApply, task.Type())
	}

	payload, err := ParseCustomRulesApplyTask(task)
	if err != nil {
		t.Fatalf("ParseCustomRulesApplyTask returned error: %v", err)
	}
	if payload.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, payload.UserID)
	}
}

func TestWakaTimeImportTaskRoundTripsPayload(t *testing.T) {
	userID := uuid.New()
	task, err := NewWakaTimeImportTask(userID, []HeartbeatImportPayload{
		{Entity: "/tmp/main.go", Type: "file", Time: 123, Project: "stint"},
	}, services.HeartbeatDefaults{
		Plugin:          "wakatime",
		PluginVersion:   "v1.102.1",
		Editor:          "vscode",
		EditorVersion:   "1.89.0",
		OperatingSystem: "linux",
		Architecture:    "amd64",
	})
	if err != nil {
		t.Fatalf("NewWakaTimeImportTask returned error: %v", err)
	}
	if task.Type() != TypeWakaTimeImport {
		t.Fatalf("expected task type %q, got %q", TypeWakaTimeImport, task.Type())
	}

	payload, err := ParseWakaTimeImportTask(task)
	if err != nil {
		t.Fatalf("ParseWakaTimeImportTask returned error: %v", err)
	}
	if payload.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, payload.UserID)
	}
	if len(payload.Heartbeats) != 1 || payload.Heartbeats[0].Project != "stint" {
		t.Fatalf("unexpected heartbeats: %#v", payload.Heartbeats)
	}
	if payload.DefaultEditor != "vscode" || payload.DefaultOperatingSystem != "linux" {
		t.Fatalf("unexpected defaults: %#v", payload)
	}
	if payload.DefaultPlugin != "wakatime" || payload.DefaultPluginVersion != "v1.102.1" || payload.DefaultEditorVersion != "1.89.0" || payload.DefaultArchitecture != "amd64" {
		t.Fatalf("unexpected user agent defaults: %#v", payload)
	}
}

func TestHeartbeatsPurgeTaskRoundTripsPayload(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	task, err := NewHeartbeatsPurgeTask(365, now)
	if err != nil {
		t.Fatalf("NewHeartbeatsPurgeTask returned error: %v", err)
	}
	if task.Type() != TypeHeartbeatsPurge {
		t.Fatalf("expected task type %q, got %q", TypeHeartbeatsPurge, task.Type())
	}

	payload, err := ParseHeartbeatsPurgeTask(task)
	if err != nil {
		t.Fatalf("ParseHeartbeatsPurgeTask returned error: %v", err)
	}
	if payload.RetentionDays != 365 {
		t.Fatalf("expected retention days 365, got %d", payload.RetentionDays)
	}
	if payload.NowUnix != now.Unix() {
		t.Fatalf("expected now unix %d, got %d", now.Unix(), payload.NowUnix)
	}
}

func TestHeartbeatsPurgeCutoffUsesRetentionDays(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	cutoff, ok := HeartbeatsPurgeCutoff(HeartbeatsPurgePayload{RetentionDays: 30, NowUnix: now.Unix()})
	if !ok {
		t.Fatal("expected positive retention to produce cutoff")
	}
	want := float64(now.AddDate(0, 0, -30).Unix())
	if cutoff != want {
		t.Fatalf("expected cutoff %.0f, got %.0f", want, cutoff)
	}
}

func TestHeartbeatsPurgeCutoffDisablesNonPositiveRetention(t *testing.T) {
	if _, ok := HeartbeatsPurgeCutoff(HeartbeatsPurgePayload{RetentionDays: 0}); ok {
		t.Fatal("expected zero retention to disable purge")
	}
}

func TestLeaderboardUpdateTaskRoundTripsPayload(t *testing.T) {
	task, err := NewLeaderboardUpdateTask("last_30_days")
	if err != nil {
		t.Fatalf("NewLeaderboardUpdateTask returned error: %v", err)
	}
	if task.Type() != TypeLeaderboardUpdate {
		t.Fatalf("expected task type %q, got %q", TypeLeaderboardUpdate, task.Type())
	}

	payload, err := ParseLeaderboardUpdateTask(task)
	if err != nil {
		t.Fatalf("ParseLeaderboardUpdateTask returned error: %v", err)
	}
	if payload.Range != "last_30_days" {
		t.Fatalf("expected last_30_days range, got %q", payload.Range)
	}
}
