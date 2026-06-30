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

	matcher := newGoalMatcher(goal)
	filtered := goalHeartbeatsInWindow(matcher, heartbeats, windowStart, windowEnd)
	filteredExternal := goalExternalDurationsInWindow(matcher, external, windowStart, windowEnd)
	actual := sumDurations(ComputeDurations(filtered, timeout, "project")) + sumDurations(ExternalDurationsInWindow(filteredExternal, "project", windowStart, windowEnd))
	if goal.ImproveByPercent != nil {
		windowDuration := windowEnd.Sub(windowStart)
		previousStart := windowStart.Add(-windowDuration)
		previousEnd := windowStart
		previousActual := sumDurations(ComputeDurations(goalHeartbeatsInWindow(matcher, heartbeats, previousStart, previousEnd), timeout, "project")) +
			sumDurations(ExternalDurationsInWindow(goalExternalDurationsInWindow(matcher, external, previousStart, previousEnd), "project", previousStart, previousEnd))
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
	sortedHeartbeats := SortedHeartbeats(heartbeats)
	projectDurations := append(ComputeDurationsFromSorted(sortedHeartbeats, timeout, "project"), ExternalDurationsAsDurations(external, "project")...)
	languageDurations := append(ComputeDurationsFromSorted(sortedHeartbeats, timeout, "language"), ExternalDurationsAsDurations(external, "language")...)
	editorDurations := ComputeDurationsFromSorted(sortedHeartbeats, timeout, "editor")
	osDurations := ComputeDurationsFromSorted(sortedHeartbeats, timeout, "operating_system")
	machineDurations := ComputeDurationsFromSorted(sortedHeartbeats, timeout, "machine")
	categoryDurations := append(ComputeDurationsFromSorted(sortedHeartbeats, timeout, "category"), ExternalDurationsAsDurations(external, "category")...)
	branchDurations := append(ComputeDurationsFromSorted(sortedHeartbeats, timeout, "branch"), ExternalDurationsAsDurations(external, "branch")...)
	dependencyDurations := ComputeDurationsFromSorted(sortedHeartbeats, timeout, "dependencies")
	total := sumDurations(projectDurations)

	start := allTimeStart(heartbeats)
	dayCount := allTimeDays(heartbeats)
	dayTotals := map[string]int{}
	dayProjectDurations := map[string][]Duration{}
	for _, duration := range projectDurations {
		day := time.Unix(int64(duration.Time), 0).In(start.Location()).Format("2006-01-02")
		dayTotals[day] += duration.DurationSeconds
		dayProjectDurations[day] = append(dayProjectDurations[day], duration)
	}
	days := make([]DailyStat, 0, dayCount)
	best := DailyStat{}
	for i := 0; i < dayCount; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		day := DailyStat{Date: date, TotalSeconds: dayTotals[date], Text: HumanDuration(dayTotals[date]), Projects: totalsByName(dayProjectDurations[date])}
		days = append(days, day)
		if day.TotalSeconds > best.TotalSeconds {
			best = day
		}
	}
	dailyAverage := 0
	if dayCount > 0 {
		dailyAverage = total / dayCount
	}

	return Stats{
		Range:               "all_time",
		TotalSeconds:        total,
		HumanReadableTotal:  HumanDuration(total),
		DailyAverageSeconds: dailyAverage,
		HumanReadableDaily:  HumanDuration(dailyAverage),
		BestDay:             best,
		Days:                days,
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

func GoalProgressDataWindow(goal Goal, now time.Time) (time.Time, time.Time) {
	start, end := goalWindow(goal.Delta, now)
	if goal.ImproveByPercent != nil {
		windowDuration := end.Sub(start)
		start = start.Add(-windowDuration)
	}
	return start, end
}

func GoalProgressDataWindowForGoals(goals []Goal, now time.Time) (time.Time, time.Time, bool) {
	var start time.Time
	var end time.Time
	hasWindow := false
	for _, goal := range goals {
		if !goal.IsEnabled {
			continue
		}
		windowStart, windowEnd := GoalProgressDataWindow(goal, now)
		if !hasWindow || windowStart.Before(start) {
			start = windowStart
		}
		if !hasWindow || windowEnd.After(end) {
			end = windowEnd
		}
		hasWindow = true
	}
	return start, end, hasWindow
}

func goalHeartbeatsInWindow(matcher goalMatcher, heartbeats []Heartbeat, windowStart, windowEnd time.Time) []Heartbeat {
	filtered := make([]Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		timestamp := time.Unix(int64(heartbeat.Time), 0).UTC()
		if timestamp.Before(windowStart) || !timestamp.Before(windowEnd) {
			continue
		}
		if matcher.ignoreDay(timestamp) {
			continue
		}
		if !matcher.matchesProject(heartbeat.Project) {
			continue
		}
		if !matcher.matchesLanguage(heartbeat.Language) {
			continue
		}
		if !matcher.matchesEditor(heartbeat.Editor) {
			continue
		}
		filtered = append(filtered, heartbeat)
	}
	return filtered
}

func goalExternalDurationsInWindow(matcher goalMatcher, external []ExternalDuration, windowStart, windowEnd time.Time) []ExternalDuration {
	if matcher.hasEditorFilter() {
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
		if matcher.ignoreDay(effectiveStart) {
			continue
		}
		if !matcher.matchesProject(duration.Project) {
			continue
		}
		if !matcher.matchesLanguage(duration.Language) {
			continue
		}
		filtered = append(filtered, duration)
	}
	return filtered
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

type goalMatcher struct {
	projects   map[string]struct{}
	languages  map[string]struct{}
	editors    map[string]struct{}
	ignoreDays map[string]struct{}
}

func newGoalMatcher(goal Goal) goalMatcher {
	return goalMatcher{
		projects:   stringSet(goal.Projects),
		languages:  stringSet(goal.Languages),
		editors:    stringSet(goal.Editors),
		ignoreDays: stringSet(goal.IgnoreDays),
	}
}

func (m goalMatcher) matchesProject(value string) bool {
	return matchesSet(m.projects, value)
}

func (m goalMatcher) matchesLanguage(value string) bool {
	return matchesSet(m.languages, value)
}

func (m goalMatcher) matchesEditor(value string) bool {
	return matchesSet(m.editors, value)
}

func (m goalMatcher) hasEditorFilter() bool {
	return len(m.editors) > 0
}

func (m goalMatcher) ignoreDay(timestamp time.Time) bool {
	if len(m.ignoreDays) == 0 {
		return false
	}
	_, ok := m.ignoreDays[strings.ToLower(timestamp.UTC().Weekday().String())]
	return ok
}

func matchesSet(allowed map[string]struct{}, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimSpace(value))]
	return ok
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	return out
}
