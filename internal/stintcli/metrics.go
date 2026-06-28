package stintcli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"time"
)

func startMetricsProfiling() (func(), error) {
	metricsDir := filepath.Join(wakaResourcesDir(), "metrics")
	if err := os.MkdirAll(metricsDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create metrics folder: %w", err)
	}

	now := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	cpuFile, err := os.Create(filepath.Join(metricsDir, "cpu_"+now+".profile")) //nolint:gosec // User-local profiling output.
	if err != nil {
		return nil, fmt.Errorf("failed to create cpu profile file: %w", err)
	}
	cpuProfilingStarted := pprof.StartCPUProfile(cpuFile) == nil

	memFile, err := os.Create(filepath.Join(metricsDir, "mem_"+now+".profile")) //nolint:gosec // User-local profiling output.
	if err != nil {
		if cpuProfilingStarted {
			pprof.StopCPUProfile()
		}
		_ = cpuFile.Close()
		return nil, fmt.Errorf("failed to create mem profile file: %w", err)
	}
	_ = pprof.WriteHeapProfile(memFile)

	return func() {
		if cpuProfilingStarted {
			pprof.StopCPUProfile()
		}
		_ = cpuFile.Close()
		_ = memFile.Close()
	}, nil
}

func metricsRequested(args []string) bool {
	opts, err := diagnosticOptions(args)
	if err != nil {
		return false
	}
	return opts.Metrics
}
