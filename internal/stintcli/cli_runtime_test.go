package stintcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestVersionMetadataCanBeInjectedAtBuildTime(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := versionValue, commitValue, buildDateValue
	versionValue = "1.2.3"
	commitValue = "abc123"
	buildDateValue = "2026-06-28T12:00:00Z"
	t.Cleanup(func() {
		versionValue = oldVersion
		commitValue = oldCommit
		buildDateValue = oldBuildDate
	})

	if Version() != "1.2.3" {
		t.Fatalf("Version() = %q", Version())
	}
	output := verboseVersion()
	for _, want := range []string{"Version: 1.2.3", "Commit: abc123", "Built: 2026-06-28T12:00:00Z"} {
		if !strings.Contains(output, want) {
			t.Fatalf("verbose version output missing %q: %q", want, output)
		}
	}
}

func TestUserAgentPlatformIncludesKernelWhenAvailable(t *testing.T) {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		t.Skip("uname unavailable")
	}
	kernel := strings.TrimSpace(string(out))
	if kernel == "" {
		t.Skip("uname returned empty kernel")
	}
	want := runtime.GOOS + "-" + kernel + "-" + runtime.GOARCH
	if got := userAgentPlatform(); got != want {
		t.Fatalf("userAgentPlatform() = %q, want %q", got, want)
	}
}

func TestStartMetricsProfilingContinuesWhenCPUAlreadyProfiling(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	cpuProfile, err := os.Create(filepath.Join(t.TempDir(), "existing-cpu.profile"))
	if err != nil {
		t.Fatal(err)
	}
	defer cpuProfile.Close()
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	stop, err := startMetricsProfiling()
	if err != nil {
		t.Fatal(err)
	}
	stop()

	entries, err := os.ReadDir(filepath.Join(dir, "metrics"))
	if err != nil {
		t.Fatal(err)
	}
	var cpu, mem int
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "cpu_") && strings.HasSuffix(entry.Name(), ".profile") {
			cpu++
		}
		if strings.HasPrefix(entry.Name(), "mem_") && strings.HasSuffix(entry.Name(), ".profile") {
			mem++
		}
	}
	if cpu != 1 || mem != 1 {
		t.Fatalf("profiles cpu=%d mem=%d entries=%#v", cpu, mem, entries)
	}
}

func TestLocalTimezoneNameFallsBackToUTC(t *testing.T) {
	t.Setenv("TZ", "Not/AZone")
	if got := localTimezoneName(); got == "" {
		t.Fatalf("expected fallback timezone")
	}
}

func TestMachineNameUsesGitpodFallback(t *testing.T) {
	t.Setenv("GITPOD_WORKSPACE_ID", "workspace-id")
	if got := machineName(""); got != "Gitpod" {
		t.Fatalf("machineName = %q", got)
	}
	if got := machineName("explicit-host"); got != "explicit-host" {
		t.Fatalf("machineName override = %q", got)
	}
}
