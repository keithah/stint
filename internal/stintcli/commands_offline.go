package stintcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

func runOffline(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint offline count|print|sync")
	}
	opts, err := parseCommon(args[1:])
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	switch args[0] {
	case "count":
		count, err := CountQueue(opts.QueuePath)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, count)
		return nil
	case "print":
		return printQueue(stdout, opts.QueuePath, opts.PrintOffline)
	case "sync":
		limit := opts.SyncOffline
		return syncOffline(stdout, opts, limit)
	default:
		return fmt.Errorf("unknown offline command %q", args[0])
	}
}

func printQueue(stdout io.Writer, path string, limit int) error {
	if limit <= 0 {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode([]Heartbeat{})
	}
	heartbeats, err := ReadQueue(path, limit)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(heartbeats)
}

func syncOffline(stdout io.Writer, opts Options, limit int) error {
	if limit < 0 {
		limit = 0
	}
	totalSynced := 0
	if opts.LegacyQueuePath != "" && opts.LegacyQueuePath != opts.QueuePath && fileExists(opts.LegacyQueuePath) {
		synced, err := syncOfflineQueue(opts, opts.LegacyQueuePath, limit)
		if err != nil {
			return err
		}
		totalSynced += synced
		if err := os.Remove(opts.LegacyQueuePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	synced, err := syncOfflineQueue(opts, opts.QueuePath, limit)
	if err != nil {
		return err
	}
	totalSynced += synced
	if totalSynced > 0 {
		fmt.Fprintf(stdout, "synced=%d\n", totalSynced)
	}
	return nil
}

func syncOfflineQueue(opts Options, queuePath string, limit int) (int, error) {
	if _, err := DeleteQueueDuplicates(queuePath); err != nil {
		return 0, err
	}
	originalCount, err := CountQueue(queuePath)
	if err != nil {
		return 0, err
	}
	if limit > 0 && originalCount > limit {
		originalCount = limit
	}
	totalSynced := 0
	remaining := originalCount
	for remaining > 0 {
		batchLimit := defaultHeartbeatLimit
		if remaining < batchLimit {
			batchLimit = remaining
		}
		heartbeats, err := ReadQueue(queuePath, batchLimit)
		if err != nil {
			return totalSynced, err
		}
		if len(heartbeats) == 0 {
			break
		}
		if err := RemoveQueuePrefix(queuePath, len(heartbeats)); err != nil {
			return totalSynced, err
		}
		requeue, err := postOfflineHeartbeats(opts, heartbeats)
		if err != nil {
			if qerr := AppendQueue(queuePath, heartbeats); qerr != nil {
				return totalSynced, errors.Join(err, qerr)
			}
			return totalSynced, err
		}
		if len(requeue) > 0 {
			if err := AppendQueue(queuePath, requeue); err != nil {
				return totalSynced, err
			}
		}
		totalSynced += len(heartbeats)
		remaining -= len(heartbeats)
	}
	return totalSynced, nil
}
