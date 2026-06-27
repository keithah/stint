package services

import (
	"sort"
	"strings"
	"time"
)

func ComputeDurations(heartbeats []Heartbeat, timeout time.Duration, sliceBy string) []Duration {
	return ComputeDurationsFromSorted(SortedHeartbeats(heartbeats), timeout, sliceBy)
}

func SortedHeartbeats(heartbeats []Heartbeat) []Heartbeat {
	if len(heartbeats) == 0 {
		return nil
	}
	items := append([]Heartbeat(nil), heartbeats...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Time < items[j].Time
	})
	return items
}

func ComputeDurationsFromSorted(items []Heartbeat, timeout time.Duration, sliceBy string) []Duration {
	if len(items) == 0 {
		return nil
	}
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
			grouped[name] = append(grouped[name], durationRowForHeartbeat(name, sliceBy, heartbeat, seconds))
		}
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)

	var durations []Duration
	for _, name := range names {
		durations = append(durations, mergeAdjacentDurationsFromSorted(grouped[name])...)
	}

	sort.SliceStable(durations, func(i, j int) bool {
		if durations[i].Name == durations[j].Name {
			return durations[i].Time < durations[j].Time
		}
		return durations[i].Name < durations[j].Name
	})
	return durations
}

func mergeAdjacentDurationsFromSorted(items []Duration) []Duration {
	if len(items) == 0 {
		return nil
	}
	out := []Duration{items[0]}
	for _, item := range items[1:] {
		last := &out[len(out)-1]
		lastEnd := last.Time + float64(last.DurationSeconds)
		gap := time.Duration((item.Time - lastEnd) * float64(time.Second))
		if gap <= 0 {
			last.DurationSeconds = int(item.Time - last.Time + float64(item.DurationSeconds))
			mergeDurationAI(last, item)
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
	row.AIAgentCosts = map[string]float64{}
	return row
}

func durationRowForHeartbeat(name, sliceBy string, heartbeat Heartbeat, seconds int) Duration {
	row := durationRow(name, sliceBy, heartbeat.Time, seconds)
	applyHeartbeatAIToDuration(&row, heartbeat)
	return row
}

func applyHeartbeatAIToDuration(row *Duration, heartbeat Heartbeat) {
	if !hasAIFields(heartbeat) {
		return
	}
	row.isAI = true
	row.AIAdditions = valueOrZero(heartbeat.AILineChanges)
	row.HumanAdditions = valueOrZero(heartbeat.HumanLineChanges)
	row.AIInputTokens = valueOrZero(heartbeat.AIInputTokens)
	row.AIOutputTokens = valueOrZero(heartbeat.AIOutputTokens)
	agent := aiAttributionName(heartbeat)
	if row.AIAgentCosts == nil {
		row.AIAgentCosts = map[string]float64{}
	}
	row.AIAgentCosts[agent] += 0

	promptLength := valueOrZero(heartbeat.AIPromptLength)
	if promptLength <= 0 {
		recomputeDurationPromptStats(row)
		return
	}
	row.AIPromptLengthSum = promptLength
	row.AIPromptEventsTotal = 1
	if heartbeat.AISession == "" {
		recomputeDurationPromptStats(row)
		return
	}
	row.AISessions = 1
	row.aiPromptEventsBySession = map[string]int{heartbeat.AISession: 1}
	row.aiPromptLengthTotalsBySession = map[string]int{heartbeat.AISession: promptLength}
	recomputeDurationPromptStats(row)
}

func mergeDurationAI(dst *Duration, src Duration) {
	dst.isAI = dst.isAI || src.isAI
	dst.AIAdditions += src.AIAdditions
	dst.AIDeletions += src.AIDeletions
	dst.HumanAdditions += src.HumanAdditions
	dst.HumanDeletions += src.HumanDeletions
	dst.AIInputTokens += src.AIInputTokens
	dst.AIOutputTokens += src.AIOutputTokens
	dst.AIPromptLengthSum += src.AIPromptLengthSum
	dst.AIPromptEventsTotal += src.AIPromptEventsTotal
	if dst.AIAgentCosts == nil {
		dst.AIAgentCosts = map[string]float64{}
	}
	for agent, cost := range src.AIAgentCosts {
		dst.AIAgentCosts[agent] += cost
	}
	if len(src.aiPromptEventsBySession) > 0 {
		if dst.aiPromptEventsBySession == nil {
			dst.aiPromptEventsBySession = map[string]int{}
		}
		for session, events := range src.aiPromptEventsBySession {
			dst.aiPromptEventsBySession[session] += events
		}
	}
	if len(src.aiPromptLengthTotalsBySession) > 0 {
		if dst.aiPromptLengthTotalsBySession == nil {
			dst.aiPromptLengthTotalsBySession = map[string]int{}
		}
		for session, length := range src.aiPromptLengthTotalsBySession {
			dst.aiPromptLengthTotalsBySession[session] += length
		}
	}
	recomputeDurationPromptStats(dst)
}

func recomputeDurationPromptStats(row *Duration) {
	if row.AIPromptEventsTotal > 0 {
		row.AIPromptLengthAvg = row.AIPromptLengthSum / row.AIPromptEventsTotal
	} else {
		row.AIPromptLengthAvg = 0
	}

	sessionPromptEvents := make([]int, 0, len(row.aiPromptEventsBySession))
	sessionPromptLengths := make([]int, 0, len(row.aiPromptLengthTotalsBySession))
	for _, events := range row.aiPromptEventsBySession {
		sessionPromptEvents = append(sessionPromptEvents, events)
	}
	for _, length := range row.aiPromptLengthTotalsBySession {
		sessionPromptLengths = append(sessionPromptLengths, length)
	}
	row.AISessions = len(row.aiPromptEventsBySession)
	if row.AISessions == 0 {
		row.AIPromptLengthAvgPerSession = 0
		row.AIPromptLengthMedianPerSession = 0
		row.AIPromptEventsAvgPerSession = 0
		row.AIPromptEventsMedianPerSession = 0
		return
	}
	row.AIPromptLengthAvgPerSession = sumInts(sessionPromptLengths) / row.AISessions
	row.AIPromptLengthMedianPerSession = medianInt(sessionPromptLengths)
	row.AIPromptEventsAvgPerSession = sumInts(sessionPromptEvents) / row.AISessions
	row.AIPromptEventsMedianPerSession = medianInt(sessionPromptEvents)
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
