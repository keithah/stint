package stintcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func runRoot(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		_ = fallbackSaveHeartbeatWithoutConfig(args)
		return err
	}
	opts.LogWriter = stdout
	switch {
	case opts.UserAgent:
		fmt.Fprintln(stdout, userAgent(opts.Plugin))
		return nil
	case opts.Version:
		if opts.Verbose {
			fmt.Fprintln(stdout, verboseVersion())
			return nil
		}
		fmt.Fprintln(stdout, Version())
		return nil
	case opts.ConfigReadSet:
		return writeConfigRead(stdout, opts.Config, opts.ConfigSection, opts.ConfigRead)
	case len(opts.ConfigWrite) > 0:
		return WriteConfigValues(opts.ConfigPath, opts.ConfigSection, opts.ConfigWrite)
	case opts.Today:
		return runTodayWithOptions(stdout, opts)
	case opts.TodayGoalSet:
		return runTodayGoalWithOptions(stdout, opts)
	case opts.FileExperts:
		return postFileExperts(stdout, opts, opts.Entity, opts.Project)
	case opts.EntitySet:
		return sendHeartbeat(stdin, stdout, opts)
	case opts.SyncOfflineSet:
		return syncOffline(stdout, opts, opts.SyncOffline)
	case opts.SyncAIActivity:
		return runSyncAIActivity(stdout, opts)
	case opts.OfflineCount:
		count, err := CountQueue(opts.QueuePath)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, count)
		return nil
	case opts.PrintOfflineSet:
		return printQueue(stdout, opts.QueuePath, opts.PrintOffline)
	default:
		printHelp(stdout)
		return fmt.Errorf("provide a command or one of --entity, --today, --today-goal, --file-experts, --offline-count, --print-offline-heartbeats, --sync-offline-activity, --config-read, --config-write")
	}
}

func fallbackSaveHeartbeatWithoutConfig(args []string) error {
	sanitized := withoutConfigFlag(args)
	sanitized = append(sanitized, "--config", filepath.Join(os.TempDir(), "stint-fallback-missing.cfg"))
	opts, err := parseCommon(sanitized)
	if err != nil {
		return err
	}
	if opts.Entity == "" || opts.DisableOffline {
		return nil
	}
	hb, err := BuildHeartbeat(opts)
	if err != nil {
		return err
	}
	return AppendQueue(opts.QueuePath, []Heartbeat{hb})
}

func withoutConfigFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}
