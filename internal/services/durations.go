package services

import (
	"sort"
	"strings"
	"time"
)

func ComputeDurations(heartbeats []Heartbeat, timeout time.Duration, sliceBy string) []Duration {
	if len(heartbeats) == 0 {
		return nil
	}

	items := append([]Heartbeat(nil), heartbeats...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Time < items[j].Time
	})

	grouped := map[string][]Duration{}
	for i, heartbeat := range items {
		seconds := 0
		if i+1 < len(items) {
			gap := time.Duration((items[i+1].Time - heartbeat.Time) * float64(time.Second))
			// A gap larger than the keystroke timeout is a session boundary:
			// the time is idle and counts as zero, and the next heartbeat begins
			// a new duration. This matches WakaTime, which never credits time
			// beyond the timeout. (Previously this capped the gap at the timeout
			// and still credited it, inflating totals by ~timeout per idle gap.)
			if gap > 0 && gap <= timeout {
				seconds = int(gap.Seconds())
			}
		}
		for _, name := range sliceNames(heartbeat, sliceBy) {
			grouped[name] = append(grouped[name], durationRow(name, sliceBy, heartbeat.Time, seconds))
		}
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)

	var durations []Duration
	for _, name := range names {
		durations = append(durations, mergeAdjacentDurations(grouped[name], timeout)...)
	}

	sort.SliceStable(durations, func(i, j int) bool {
		if durations[i].Name == durations[j].Name {
			return durations[i].Time < durations[j].Time
		}
		return durations[i].Name < durations[j].Name
	})
	return durations
}

func mergeAdjacentDurations(items []Duration, timeout time.Duration) []Duration {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Time < items[j].Time
	})
	out := []Duration{items[0]}
	for _, item := range items[1:] {
		last := &out[len(out)-1]
		lastEnd := last.Time + float64(last.DurationSeconds)
		gap := time.Duration((item.Time - lastEnd) * float64(time.Second))
		if gap <= 0 {
			last.DurationSeconds = int(item.Time - last.Time + float64(item.DurationSeconds))
			continue
		}
		out = append(out, item)
	}
	return out
}

func sliceNames(heartbeat Heartbeat, sliceBy string) []string {
	if sliceBy != "dependencies" {
		name := sliceName(heartbeat, sliceBy)
		if name == "" {
			return nil
		}
		return []string{name}
	}
	if heartbeat.Dependencies == "" {
		return []string{"Unknown"}
	}
	names := strings.Split(heartbeat.Dependencies, ",")
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{"Unknown"}
	}
	sort.Strings(out)
	return out
}

func sliceName(heartbeat Heartbeat, sliceBy string) string {
	switch sliceBy {
	case "language":
		if heartbeat.Language != "" {
			return heartbeat.Language
		}
		return "Other"
	case "editor":
		if heartbeat.Editor != "" {
			return heartbeat.Editor
		}
	case "operating_system":
		if heartbeat.OperatingSystem != "" {
			return heartbeat.OperatingSystem
		}
	case "machine":
		if heartbeat.MachineName != "" {
			return heartbeat.MachineName
		}
	case "category":
		if heartbeat.Category != "" {
			return heartbeat.Category
		}
	case "branch":
		if heartbeat.Branch != "" {
			return heartbeat.Branch
		}
		return ""
	case "commit":
		if hash := normalizeCommitHash(heartbeat); hash != "" {
			return hash
		}
	case "dependencies":
		if heartbeat.Dependencies != "" {
			return heartbeat.Dependencies
		}
	default:
		if heartbeat.Project != "" {
			return heartbeat.Project
		}
	}
	return "Unknown"
}

func durationRow(name, sliceBy string, start float64, seconds int) Duration {
	row := Duration{Name: name, Time: start, DurationSeconds: seconds}
	if sliceBy == "language" {
		row.Language = name
	} else {
		row.Project = name
	}
	return row
}
