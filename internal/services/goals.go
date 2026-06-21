package services

import (
	"math"
	"strings"
	"time"
)

func ComputeGoalProgress(goal Goal, heartbeats []Heartbeat, now time.Time, timeout time.Duration) GoalProgress {
	return ComputeGoalProgressWithExternalDurations(goal, heartbeats, nil, now, timeout)
}

func ComputeGoalProgressWithExternalDurations(goal Goal, heartbeats []Heartbeat, external []ExternalDuration, now time.Time, timeout time.Duration) GoalProgress {
	windowStart, windowEnd := goalWindow(goal.Delta, now)
	return ComputeGoalProgressForWindowWithExternalDurations(goal, heartbeats, external, windowStart, windowEnd, timeout)
}

func ComputeGoalProgressForWindow(goal Goal, heartbeats []Heartbeat, windowStart, windowEnd time.Time, timeout time.Duration) GoalProgress {
	return ComputeGoalProgressForWindowWithExternalDurations(goal, heartbeats, nil, windowStart, windowEnd, timeout)
}

func ComputeGoalProgressForWindowWithExternalDurations(goal Goal, heartbeats []Heartbeat, external []ExternalDuration, windowStart, windowEnd time.Time, timeout time.Duration) GoalProgress {
	target := goal.Seconds
	if target < 0 {
		target = 0
	}

	if goalSnoozed(goal, time.Now()) {
		return GoalProgress{
			Goal:             goal,
			ActualSeconds:    0,
			TargetSeconds:    target,
			Percent:          0,
			IsComplete:       false,
			HumanReadable:    "0 secs",
			TargetReadable:   HumanDuration(target),
			RemainingSeconds: target,
			IsSnoozed:        true,
		}
	}

	filtered := goalHeartbeatsInWindow(goal, heartbeats, windowStart, windowEnd)
	filteredExternal := goalExternalDurationsInWindow(goal, external, windowStart, windowEnd)
	actual := sumDurations(ComputeDurations(filtered, timeout, "project")) + sumDurations(ExternalDurationsInWindow(filteredExternal, "project", windowStart, windowEnd))
	if goal.ImproveByPercent != nil {
		windowDuration := windowEnd.Sub(windowStart)
		previousStart := windowStart.Add(-windowDuration)
		previousEnd := windowStart
		previousActual := sumDurations(ComputeDurations(goalHeartbeatsInWindow(goal, heartbeats, previousStart, previousEnd), timeout, "project")) +
			sumDurations(ExternalDurationsInWindow(goalExternalDurationsInWindow(goal, external, previousStart, previousEnd), "project", previousStart, previousEnd))
		if previousActual > 0 {
			target = previousActual + int(math.Ceil(float64(previousActual)*(*goal.ImproveByPercent)/100))
			if target < 0 {
				target = 0
			}
		}
	}
	if goal.IgnoreZeroDays && actual == 0 {
		return GoalProgress{
			Goal:             goal,
			ActualSeconds:    actual,
			TargetSeconds:    target,
			Percent:          100,
			IsComplete:       true,
			HumanReadable:    HumanDuration(actual),
			TargetReadable:   HumanDuration(target),
			RemainingSeconds: 0,
			IsIgnored:        true,
		}
	}
	percent := 0
	if target > 0 {
		percent = (actual * 100) / target
		if percent > 100 {
			percent = 100
		}
	}
	remaining := target - actual
	if remaining < 0 {
		remaining = 0
	}
	complete := actual >= target
	if goal.IsInverse {
		complete = actual <= target
	}
	return GoalProgress{
		Goal:             goal,
		ActualSeconds:    actual,
		TargetSeconds:    target,
		Percent:          percent,
		IsComplete:       complete,
		HumanReadable:    HumanDuration(actual),
		TargetReadable:   HumanDuration(target),
		RemainingSeconds: remaining,
	}
}

func ComputeAllTimeStats(heartbeats []Heartbeat, timeout time.Duration) Stats {
	return ComputeAllTimeStatsWithAICosts(heartbeats, timeout, nil)
}

func ComputeAllTimeStatsWithAICosts(heartbeats []Heartbeat, timeout time.Duration, costs map[string]AICostRate) Stats {
	return ComputeAllTimeStatsWithExternalDurationsAndAICosts(heartbeats, nil, timeout, costs)
}

func ComputeAllTimeStatsWithExternalDurations(heartbeats []Heartbeat, external []ExternalDuration, timeout time.Duration) Stats {
	return ComputeAllTimeStatsWithExternalDurationsAndAICosts(heartbeats, external, timeout, nil)
}

func ComputeAllTimeStatsWithExternalDurationsAndAICosts(heartbeats []Heartbeat, external []ExternalDuration, timeout time.Duration, costs map[string]AICostRate) Stats {
	projectDurations := append(ComputeDurations(heartbeats, timeout, "project"), ExternalDurationsAsDurations(external, "project")...)
	languageDurations := append(ComputeDurations(heartbeats, timeout, "language"), ExternalDurationsAsDurations(external, "language")...)
	editorDurations := ComputeDurations(heartbeats, timeout, "editor")
	osDurations := ComputeDurations(heartbeats, timeout, "operating_system")
	machineDurations := ComputeDurations(heartbeats, timeout, "machine")
	categoryDurations := append(ComputeDurations(heartbeats, timeout, "category"), ExternalDurationsAsDurations(external, "category")...)
	branchDurations := append(ComputeDurations(heartbeats, timeout, "branch"), ExternalDurationsAsDurations(external, "branch")...)
	dependencyDurations := ComputeDurations(heartbeats, timeout, "dependencies")
	total := sumDurations(projectDurations)
	return Stats{
		Range:               "all_time",
		TotalSeconds:        total,
		HumanReadableTotal:  HumanDuration(total),
		DailyAverageSeconds: 0,
		HumanReadableDaily:  "0 secs",
		Projects:            totalsByName(projectDurations),
		Languages:           totalsByName(languageDurations),
		Editors:             totalsByName(editorDurations),
		OperatingSystems:    totalsByName(osDurations),
		Machines:            totalsByName(machineDurations),
		Categories:          totalsByName(categoryDurations),
		Branches:            totalsByName(branchDurations),
		Dependencies:        totalsByName(dependencyDurations),
		AI:                  computeAIMetrics(heartbeats, projectDurations, allTimeStart(heartbeats), allTimeDays(heartbeats), costs),
		IsUpToDate:          true,
		PercentCalculated:   100,
	}
}

func allTimeStart(heartbeats []Heartbeat) time.Time {
	if len(heartbeats) == 0 {
		return beginningOfDay(time.Now().UTC())
	}
	oldest := time.Unix(int64(heartbeats[0].Time), 0).UTC()
	for _, heartbeat := range heartbeats[1:] {
		timestamp := time.Unix(int64(heartbeat.Time), 0).UTC()
		if timestamp.Before(oldest) {
			oldest = timestamp
		}
	}
	return beginningOfDay(oldest)
}

func allTimeDays(heartbeats []Heartbeat) int {
	if len(heartbeats) == 0 {
		return 1
	}
	start := allTimeStart(heartbeats)
	newest := time.Unix(int64(heartbeats[0].Time), 0).UTC()
	for _, heartbeat := range heartbeats[1:] {
		timestamp := time.Unix(int64(heartbeat.Time), 0).UTC()
		if timestamp.After(newest) {
			newest = timestamp
		}
	}
	return daysBetween(start, beginningOfDay(newest).AddDate(0, 0, 1))
}

func goalWindow(delta string, now time.Time) (time.Time, time.Time) {
	end := beginningOfDay(now.UTC()).AddDate(0, 0, 1)
	if delta == "week" {
		weekday := int(now.UTC().Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := beginningOfDay(now.UTC()).AddDate(0, 0, -(weekday - 1))
		return start, start.AddDate(0, 0, 7)
	}
	return beginningOfDay(now.UTC()), end
}

func GoalEvaluationWindow(delta string, now time.Time) (time.Time, time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if delta == "week" {
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		thisWeekStart := today.AddDate(0, 0, -(weekday - 1))
		previousWeekStart := thisWeekStart.AddDate(0, 0, -7)
		return previousWeekStart.UTC(), thisWeekStart.UTC()
	}
	return today.AddDate(0, 0, -1).UTC(), today.UTC()
}

func goalHeartbeatsInWindow(goal Goal, heartbeats []Heartbeat, windowStart, windowEnd time.Time) []Heartbeat {
	filtered := make([]Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		timestamp := time.Unix(int64(heartbeat.Time), 0).UTC()
		if timestamp.Before(windowStart) || !timestamp.Before(windowEnd) {
			continue
		}
		if ignoreGoalDay(goal.IgnoreDays, timestamp) {
			continue
		}
		if !matchesAny(goal.Projects, heartbeat.Project) {
			continue
		}
		if !matchesAny(goal.Languages, heartbeat.Language) {
			continue
		}
		if !matchesAny(goal.Editors, heartbeat.Editor) {
			continue
		}
		filtered = append(filtered, heartbeat)
	}
	return filtered
}

func goalExternalDurationsInWindow(goal Goal, external []ExternalDuration, windowStart, windowEnd time.Time) []ExternalDuration {
	if len(goal.Editors) > 0 {
		return nil
	}
	filtered := make([]ExternalDuration, 0, len(external))
	for _, duration := range external {
		start := time.Unix(int64(duration.StartTime), 0).UTC()
		end := time.Unix(int64(duration.EndTime), 0).UTC()
		if !start.Before(windowEnd) || !end.After(windowStart) {
			continue
		}
		effectiveStart := start
		if effectiveStart.Before(windowStart) {
			effectiveStart = windowStart
		}
		if ignoreGoalDay(goal.IgnoreDays, effectiveStart) {
			continue
		}
		if !matchesAny(goal.Projects, duration.Project) {
			continue
		}
		if !matchesAny(goal.Languages, duration.Language) {
			continue
		}
		filtered = append(filtered, duration)
	}
	return filtered
}

func ignoreGoalDay(ignoreDays []string, timestamp time.Time) bool {
	weekday := strings.ToLower(timestamp.UTC().Weekday().String())
	for _, day := range ignoreDays {
		if strings.ToLower(day) == weekday {
			return true
		}
	}
	return false
}

func goalSnoozed(goal Goal, now time.Time) bool {
	if !goal.IsSnoozed {
		return false
	}
	if goal.SnoozeUntil == "" {
		return true
	}
	until, err := time.Parse(time.RFC3339, goal.SnoozeUntil)
	return err != nil || now.Before(until)
}

func matchesAny(allowed []string, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if item == value {
			return true
		}
	}
	return false
}
