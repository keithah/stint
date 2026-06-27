package services

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type StatsWindow struct {
	Range string
	Start time.Time
	End   time.Time
	Days  int
}

type AICostRate struct {
	InputCostPerMillionCents  int `json:"input_cost_per_million_cents"`
	OutputCostPerMillionCents int `json:"output_cost_per_million_cents"`
}

func ComputeLast7DaysStats(heartbeats []Heartbeat, now time.Time, timeout time.Duration) Stats {
	stats, _, _ := ComputeStatsForRange(heartbeats, now, timeout, "last_7_days")
	return stats
}

func ComputeStatsForRange(heartbeats []Heartbeat, now time.Time, timeout time.Duration, rangeName string) (Stats, StatsWindow, error) {
	return ComputeStatsForRangeWithAICosts(heartbeats, now, timeout, rangeName, nil)
}

func ComputeStatsForRangeWithAICosts(heartbeats []Heartbeat, now time.Time, timeout time.Duration, rangeName string, costs map[string]AICostRate) (Stats, StatsWindow, error) {
	return ComputeStatsForRangeWithExternalDurationsAndAICosts(heartbeats, nil, now, timeout, rangeName, costs)
}

func ComputeStatsForRangeWithExternalDurations(heartbeats []Heartbeat, external []ExternalDuration, now time.Time, timeout time.Duration, rangeName string) (Stats, StatsWindow, error) {
	return ComputeStatsForRangeWithExternalDurationsAndAICosts(heartbeats, external, now, timeout, rangeName, nil)
}

func ComputeStatsForRangeWithExternalDurationsAndAICosts(heartbeats []Heartbeat, external []ExternalDuration, now time.Time, timeout time.Duration, rangeName string, costs map[string]AICostRate) (Stats, StatsWindow, error) {
	window, err := WindowForRange(now, rangeName)
	if err != nil {
		return Stats{}, StatsWindow{}, err
	}

	var inRange []Heartbeat
	for _, heartbeat := range heartbeats {
		timestamp := time.Unix(int64(heartbeat.Time), 0).UTC()
		if !timestamp.Before(window.Start) && timestamp.Before(window.End) {
			inRange = append(inRange, heartbeat)
		}
	}

	sortedInRange := SortedHeartbeats(inRange)
	projectDurations := append(ComputeDurationsFromSorted(sortedInRange, timeout, "project"), ExternalDurationsInWindow(external, "project", window.Start, window.End)...)
	languageDurations := append(ComputeDurationsFromSorted(sortedInRange, timeout, "language"), ExternalDurationsInWindow(external, "language", window.Start, window.End)...)
	editorDurations := ComputeDurationsFromSorted(sortedInRange, timeout, "editor")
	osDurations := ComputeDurationsFromSorted(sortedInRange, timeout, "operating_system")
	machineDurations := ComputeDurationsFromSorted(sortedInRange, timeout, "machine")
	categoryDurations := append(ComputeDurationsFromSorted(sortedInRange, timeout, "category"), ExternalDurationsInWindow(external, "category", window.Start, window.End)...)
	branchDurations := append(ComputeDurationsFromSorted(sortedInRange, timeout, "branch"), ExternalDurationsInWindow(external, "branch", window.Start, window.End)...)
	dependencyDurations := ComputeDurationsFromSorted(sortedInRange, timeout, "dependencies")
	total := sumDurations(projectDurations)

	dayTotals := map[string]int{}
	dayProjectDurations := map[string][]Duration{}
	for _, duration := range projectDurations {
		day := time.Unix(int64(duration.Time), 0).In(window.Start.Location()).Format("2006-01-02")
		dayTotals[day] += duration.DurationSeconds
		dayProjectDurations[day] = append(dayProjectDurations[day], duration)
	}

	days := make([]DailyStat, 0, window.Days)
	best := DailyStat{}
	for i := 0; i < window.Days; i++ {
		date := window.Start.AddDate(0, 0, i).Format("2006-01-02")
		day := DailyStat{Date: date, TotalSeconds: dayTotals[date], Text: HumanDuration(dayTotals[date]), Projects: totalsByName(dayProjectDurations[date])}
		days = append(days, day)
		if day.TotalSeconds > best.TotalSeconds {
			best = day
		}
	}

	return Stats{
		Range:               window.Range,
		TotalSeconds:        total,
		HumanReadableTotal:  HumanDuration(total),
		DailyAverageSeconds: total / window.Days,
		HumanReadableDaily:  HumanDuration(total / window.Days),
		BestDay:             best,
		Days:                days,
		Hourly:              ComputeHourlyTimeline(projectDurations, languageDurations),
		Projects:            totalsByName(projectDurations),
		Languages:           totalsByName(languageDurations),
		Editors:             totalsByName(editorDurations),
		OperatingSystems:    totalsByName(osDurations),
		Machines:            totalsByName(machineDurations),
		Categories:          totalsByName(categoryDurations),
		Branches:            totalsByName(branchDurations),
		Dependencies:        totalsByName(dependencyDurations),
		AI:                  computeAIMetrics(inRange, projectDurations, window.Start, window.Days, costs),
		ProjectAI:           computeProjectAIMetrics(inRange, projectDurations, costs),
		IsUpToDate:          true,
		PercentCalculated:   100,
	}, window, nil
}

func ExternalDurationsInWindow(external []ExternalDuration, sliceBy string, start, end time.Time) []Duration {
	if len(external) == 0 {
		return nil
	}
	startUnix := float64(start.Unix())
	endUnix := float64(end.Unix())
	rows := make([]Duration, 0, len(external))
	for _, externalDuration := range external {
		clippedStart := externalDuration.StartTime
		clippedEnd := externalDuration.EndTime
		if clippedStart < startUnix {
			clippedStart = startUnix
		}
		if clippedEnd > endUnix {
			clippedEnd = endUnix
		}
		seconds := int(clippedEnd - clippedStart)
		if seconds <= 0 {
			continue
		}
		rows = append(rows, durationRow(externalSliceName(externalDuration, sliceBy), sliceBy, clippedStart, seconds))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Name == rows[j].Name {
			return rows[i].Time < rows[j].Time
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func ExternalDurationsAsDurations(external []ExternalDuration, sliceBy string) []Duration {
	if len(external) == 0 {
		return nil
	}
	rows := make([]Duration, 0, len(external))
	for _, externalDuration := range external {
		seconds := int(externalDuration.EndTime - externalDuration.StartTime)
		if seconds <= 0 {
			continue
		}
		rows = append(rows, durationRow(externalSliceName(externalDuration, sliceBy), sliceBy, externalDuration.StartTime, seconds))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Name == rows[j].Name {
			return rows[i].Time < rows[j].Time
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func WindowForRange(now time.Time, rangeName string) (StatsWindow, error) {
	end := beginningOfDay(now).AddDate(0, 0, 1)
	switch rangeName {
	case "", "last_7_days":
		return StatsWindow{Range: "last_7_days", Start: end.AddDate(0, 0, -7), End: end, Days: 7}, nil
	case "last_30_days":
		return StatsWindow{Range: "last_30_days", Start: end.AddDate(0, 0, -30), End: end, Days: 30}, nil
	case "last_6_months":
		start := end.AddDate(0, -6, 0)
		return StatsWindow{Range: "last_6_months", Start: start, End: end, Days: daysBetween(start, end)}, nil
	case "last_year":
		start := end.AddDate(-1, 0, 0)
		return StatsWindow{Range: "last_year", Start: start, End: end, Days: daysBetween(start, end)}, nil
	default:
		if window, ok := calendarWindowForRange(rangeName); ok {
			return window, nil
		}
		return StatsWindow{}, errors.New("unsupported stats range")
	}
}

func calendarWindowForRange(rangeName string) (StatsWindow, bool) {
	if len(rangeName) == len("2006") {
		start, err := time.Parse("2006", rangeName)
		if err != nil {
			return StatsWindow{}, false
		}
		start = start.UTC()
		end := start.AddDate(1, 0, 0)
		return StatsWindow{Range: rangeName, Start: start, End: end, Days: daysBetween(start, end)}, true
	}
	if len(rangeName) == len("2006-01") {
		start, err := time.Parse("2006-01", rangeName)
		if err != nil {
			return StatsWindow{}, false
		}
		start = start.UTC()
		end := start.AddDate(0, 1, 0)
		return StatsWindow{Range: rangeName, Start: start, End: end, Days: daysBetween(start, end)}, true
	}
	return StatsWindow{}, false
}

func ComputeStatusBarToday(heartbeats []Heartbeat, now time.Time, timeout time.Duration) StatusBarStats {
	start := beginningOfDay(now)
	return ComputeStatusBarForWindowWithExternalDurations(heartbeats, nil, start, start.AddDate(0, 0, 1), timeout)
}

func ComputeStatusBarForWindowWithExternalDurations(heartbeats []Heartbeat, external []ExternalDuration, start, end time.Time, timeout time.Duration) StatusBarStats {
	inWindow := make([]Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		timestamp := time.Unix(int64(heartbeat.Time), 0).UTC()
		if !timestamp.Before(start) && timestamp.Before(end) {
			inWindow = append(inWindow, heartbeat)
		}
	}
	projectDurations := append(ComputeDurations(inWindow, timeout, "project"), ExternalDurationsInWindow(external, "project", start, end)...)
	languageDurations := append(ComputeDurations(inWindow, timeout, "language"), ExternalDurationsInWindow(external, "language", start, end)...)
	projectTotals := totalsByName(projectDurations)
	languageTotals := totalsByName(languageDurations)
	total := sumDurations(projectDurations)
	status := StatusBarStats{
		TotalSeconds:      total,
		GrandTotalText:    HumanDuration(total),
		Range:             "today",
		PercentCalculated: 100,
	}
	if len(projectTotals) > 0 {
		status.Project = projectTotals[0].Name
		status.ProjectSeconds = projectTotals[0].TotalSeconds
		status.ProjectText = projectTotals[0].Text
	}
	if len(languageTotals) > 0 {
		status.Language = languageTotals[0].Name
		status.LanguageSeconds = languageTotals[0].TotalSeconds
		status.LanguageText = languageTotals[0].Text
	}
	return status
}

func ComputeWeekdayStats(days []DailyStat) []WeekdayStat {
	rows := []WeekdayStat{
		{Name: "Monday", Day: 1},
		{Name: "Tuesday", Day: 2},
		{Name: "Wednesday", Day: 3},
		{Name: "Thursday", Day: 4},
		{Name: "Friday", Day: 5},
		{Name: "Saturday", Day: 6},
		{Name: "Sunday", Day: 7},
	}
	for _, day := range days {
		date, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			continue
		}
		index := int(date.Weekday()) - 1
		if index < 0 {
			index = 6
		}
		rows[index].TotalSeconds += day.TotalSeconds
		if day.TotalSeconds > 0 {
			rows[index].ActiveDays++
		}
	}
	for index := range rows {
		row := &rows[index]
		row.Text = HumanDuration(row.TotalSeconds)
		if row.ActiveDays > 0 {
			row.AverageSeconds = row.TotalSeconds / row.ActiveDays
		}
		row.AverageText = HumanDuration(row.AverageSeconds)
	}
	return rows
}

func ComputeDailyAverageTrend(days []DailyStat) []DailyAverageTrendStat {
	rows := make([]DailyAverageTrendStat, 0, len(days))
	total := 0
	for index, day := range days {
		total += day.TotalSeconds
		dayCount := index + 1
		average := 0
		if dayCount > 0 {
			average = total / dayCount
		}
		rows = append(rows, DailyAverageTrendStat{
			Date:           day.Date,
			TotalSeconds:   day.TotalSeconds,
			Text:           HumanDuration(day.TotalSeconds),
			AverageSeconds: average,
			AverageText:    HumanDuration(average),
			DayCount:       dayCount,
		})
	}
	return rows
}

func ComputeHourlyTimeline(projectDurations, languageDurations []Duration) []HourlyStat {
	rows := make([]HourlyStat, 24)
	projectTotals := make([]map[string]int, 24)
	languageTotals := make([]map[string]int, 24)
	for hour := 0; hour < 24; hour++ {
		rows[hour] = HourlyStat{Hour: hour, Label: fmt.Sprintf("%02d:00", hour)}
		projectTotals[hour] = map[string]int{}
		languageTotals[hour] = map[string]int{}
	}

	addDurationParts(projectDurations, func(hour int, name string, seconds int) {
		projectTotals[hour][name] += seconds
		rows[hour].TotalSeconds += seconds
	})
	addDurationParts(languageDurations, func(hour int, name string, seconds int) {
		languageTotals[hour][name] += seconds
	})

	for hour := range rows {
		rows[hour].Text = HumanDuration(rows[hour].TotalSeconds)
		rows[hour].Projects = sliceTotalsFromMap(projectTotals[hour])
		rows[hour].Languages = sliceTotalsFromMap(languageTotals[hour])
	}
	return rows
}

func addDurationParts(durations []Duration, add func(hour int, name string, seconds int)) {
	for _, duration := range durations {
		if duration.DurationSeconds <= 0 {
			continue
		}
		name := duration.Name
		if name == "" {
			name = "Unknown"
		}
		start := time.Unix(int64(duration.Time), 0).UTC()
		end := start.Add(time.Duration(duration.DurationSeconds) * time.Second)
		cursor := start
		for cursor.Before(end) {
			nextHour := cursor.Truncate(time.Hour).Add(time.Hour)
			partEnd := nextHour
			if end.Before(partEnd) {
				partEnd = end
			}
			seconds := int(partEnd.Sub(cursor).Seconds())
			if seconds > 0 {
				add(cursor.Hour(), name, seconds)
			}
			cursor = partEnd
		}
	}
}

func totalsByName(durations []Duration) []SliceTotal {
	totals := map[string]int{}
	for _, duration := range durations {
		totals[duration.Name] += duration.DurationSeconds
	}
	rows := make([]SliceTotal, 0, len(totals))
	for name, seconds := range totals {
		rows = append(rows, SliceTotal{Name: name, TotalSeconds: seconds, Text: HumanDuration(seconds)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalSeconds == rows[j].TotalSeconds {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].TotalSeconds > rows[j].TotalSeconds
	})
	return rows
}

func sliceTotalsFromMap(totals map[string]int) []SliceTotal {
	rows := make([]SliceTotal, 0, len(totals))
	for name, seconds := range totals {
		rows = append(rows, SliceTotal{Name: name, TotalSeconds: seconds, Text: HumanDuration(seconds)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalSeconds == rows[j].TotalSeconds {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].TotalSeconds > rows[j].TotalSeconds
	})
	return rows
}

func computeAIMetrics(heartbeats []Heartbeat, durations []Duration, start time.Time, days int, costs map[string]AICostRate) AIMetrics {
	metrics := AIMetrics{}
	agentTotals := map[string]*AIStat{}
	dayTotals := map[string]*AIStat{}
	agentCosts := map[string]*aiCostTokenTotals{}
	dayCosts := map[string]map[string]*aiCostTokenTotals{}
	costPeriods := map[string]*aiCostPeriodTokenTotals{}
	sessions := map[string]struct{}{}
	agentSessions := map[string]map[string]struct{}{}
	daySessions := map[string]map[string]struct{}{}
	promptLengths := []int{}
	promptEventsBySession := map[string]int{}
	promptLengthTotalsBySession := map[string]int{}
	windowEnd := start.AddDate(0, 0, days)
	dailyStart := windowEnd.AddDate(0, 0, -1)
	weeklyStart := windowEnd.AddDate(0, 0, -7)
	monthlyStart := windowEnd.AddDate(0, 0, -30)

	aiSecondsByDay := map[string]int{}
	for _, duration := range durations {
		if !duration.isAI {
			continue
		}
		day := time.Unix(int64(duration.Time), 0).In(start.Location()).Format("2006-01-02")
		aiSecondsByDay[day] += duration.DurationSeconds
	}

	for _, heartbeat := range heartbeats {
		metrics.AILineChanges += valueOrZero(heartbeat.AILineChanges)
		metrics.HumanLineChanges += valueOrZero(heartbeat.HumanLineChanges)
		metrics.AIInputTokens += valueOrZero(heartbeat.AIInputTokens)
		metrics.AIOutputTokens += valueOrZero(heartbeat.AIOutputTokens)
		promptLength := valueOrZero(heartbeat.AIPromptLength)
		metrics.AIPromptLength += promptLength
		if promptLength > 0 {
			promptLengths = append(promptLengths, promptLength)
			if heartbeat.AISession != "" {
				promptEventsBySession[heartbeat.AISession]++
				promptLengthTotalsBySession[heartbeat.AISession] += promptLength
			}
		}
		if heartbeat.AISession != "" {
			sessions[heartbeat.AISession] = struct{}{}
		}

		agent := aiAttributionName(heartbeat)
		if hasAIFields(heartbeat) {
			addAIStat(agentTotals, agent, heartbeat)
			addSession(agentSessions, agent, heartbeat.AISession)
			addAICostTokens(agentCosts, agent, valueOrZero(heartbeat.AIInputTokens), valueOrZero(heartbeat.AIOutputTokens))
			addAICostPeriodTokens(costPeriods, agent, valueOrZero(heartbeat.AIInputTokens), valueOrZero(heartbeat.AIOutputTokens), time.Unix(int64(heartbeat.Time), 0).In(start.Location()), dailyStart, weeklyStart, monthlyStart, windowEnd)
		}

		day := time.Unix(int64(heartbeat.Time), 0).In(start.Location()).Format("2006-01-02")
		if hasAIFields(heartbeat) {
			addAIStat(dayTotals, day, heartbeat)
			addSession(daySessions, day, heartbeat.AISession)
			addNestedAICostTokens(dayCosts, day, agent, valueOrZero(heartbeat.AIInputTokens), valueOrZero(heartbeat.AIOutputTokens))
		}
	}

	totalLines := metrics.AILineChanges + metrics.HumanLineChanges
	if totalLines > 0 {
		metrics.AIPercentage = (metrics.AILineChanges * 100) / totalLines
	}
	if metrics.AILineChanges > 0 {
		metrics.HumanReviewPercentage = (metrics.HumanLineChanges * 100) / metrics.AILineChanges
	}
	metrics.FollowUpEdits = metrics.HumanLineChanges
	metrics.PromptCount = len(promptLengths)
	if metrics.PromptCount > 0 {
		metrics.AveragePromptLength = metrics.AIPromptLength / metrics.PromptCount
		metrics.MedianPromptLength = medianInt(promptLengths)
	}
	metrics.SessionCount = len(sessions)
	metrics.AISessions = metrics.SessionCount
	applyWakaTimeAIAliases(&metrics, promptEventsBySession, promptLengthTotalsBySession)

	for day, seconds := range aiSecondsByDay {
		stat := dayTotals[day]
		if stat == nil {
			stat = &AIStat{Name: day}
			dayTotals[day] = stat
		}
		stat.AISeconds = seconds
	}
	for agent, sessions := range agentSessions {
		if stat := agentTotals[agent]; stat != nil {
			stat.SessionCount = len(sessions)
		}
	}
	for day, sessions := range daySessions {
		if stat := dayTotals[day]; stat != nil {
			stat.SessionCount = len(sessions)
		}
	}
	for agent, tokens := range agentCosts {
		cost := estimateAICostCentsForAgent(agent, tokens.InputTokens, tokens.OutputTokens, costs)
		if stat := agentTotals[agent]; stat != nil {
			stat.EstimatedCostCents = cost
		}
		metrics.EstimatedCostCents += cost
	}
	for day, agents := range dayCosts {
		stat := dayTotals[day]
		if stat == nil {
			stat = &AIStat{Name: day}
			dayTotals[day] = stat
		}
		for agent, tokens := range agents {
			stat.EstimatedCostCents += estimateAICostCentsForAgent(agent, tokens.InputTokens, tokens.OutputTokens, costs)
		}
	}

	metrics.Agents = sortedAIStats(agentTotals)
	applyWakaTimeAgentAliases(&metrics)
	metrics.Days = orderedDayAIStats(dayTotals, start, days)
	metrics.Costs = sortedAICostPeriods(costPeriods, costs)
	// Default to an empty (non-nil) slice so the field never serializes as JSON
	// null: ToolCosts is only populated when a usage_events bake runs, and a null
	// would crash clients that read its length. Stats paths that don't bake (e.g.
	// public/share) thus still emit [] rather than null.
	metrics.ToolCosts = []AIToolCost{}
	return metrics
}

type aiCostTokenTotals struct {
	InputTokens  int
	OutputTokens int
}

type aiCostPeriodTokenTotals struct {
	Agent         string
	DailyInput    int
	DailyOutput   int
	WeeklyInput   int
	WeeklyOutput  int
	MonthlyInput  int
	MonthlyOutput int
	TotalInput    int
	TotalOutput   int
}

func addAICostTokens(totals map[string]*aiCostTokenTotals, agent string, inputTokens, outputTokens int) {
	total := totals[agent]
	if total == nil {
		total = &aiCostTokenTotals{}
		totals[agent] = total
	}
	total.InputTokens += inputTokens
	total.OutputTokens += outputTokens
}

func addNestedAICostTokens(totals map[string]map[string]*aiCostTokenTotals, name, agent string, inputTokens, outputTokens int) {
	agents := totals[name]
	if agents == nil {
		agents = map[string]*aiCostTokenTotals{}
		totals[name] = agents
	}
	addAICostTokens(agents, agent, inputTokens, outputTokens)
}

func addAICostPeriodTokens(periods map[string]*aiCostPeriodTokenTotals, agent string, inputTokens, outputTokens int, timestamp, dailyStart, weeklyStart, monthlyStart, windowEnd time.Time) {
	if inputTokens == 0 && outputTokens == 0 || !timestamp.Before(windowEnd) {
		return
	}
	period := periods[agent]
	if period == nil {
		period = &aiCostPeriodTokenTotals{Agent: agent}
		periods[agent] = period
	}
	period.TotalInput += inputTokens
	period.TotalOutput += outputTokens
	if !timestamp.Before(monthlyStart) {
		period.MonthlyInput += inputTokens
		period.MonthlyOutput += outputTokens
	}
	if !timestamp.Before(weeklyStart) {
		period.WeeklyInput += inputTokens
		period.WeeklyOutput += outputTokens
	}
	if !timestamp.Before(dailyStart) {
		period.DailyInput += inputTokens
		period.DailyOutput += outputTokens
	}
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func computeProjectAIMetrics(heartbeats []Heartbeat, durations []Duration, costs map[string]AICostRate) []AIStat {
	projectTotals := map[string]*AIStat{}
	projectSessions := map[string]map[string]struct{}{}
	projectCosts := map[string]map[string]*aiCostTokenTotals{}

	for _, duration := range durations {
		if !duration.isAI {
			continue
		}
		stat := projectTotals[duration.Name]
		if stat == nil {
			stat = &AIStat{Name: duration.Name}
			projectTotals[duration.Name] = stat
		}
		stat.AISeconds += duration.DurationSeconds
	}

	for _, heartbeat := range heartbeats {
		project := heartbeat.Project
		if project == "" {
			project = "Unknown"
		}
		stat := projectTotals[project]
		if stat == nil {
			stat = &AIStat{Name: project}
			projectTotals[project] = stat
		}

		stat.AILineChanges += valueOrZero(heartbeat.AILineChanges)
		stat.HumanLineChanges += valueOrZero(heartbeat.HumanLineChanges)
		stat.AIInputTokens += valueOrZero(heartbeat.AIInputTokens)
		stat.AIOutputTokens += valueOrZero(heartbeat.AIOutputTokens)
		stat.AIPromptLength += valueOrZero(heartbeat.AIPromptLength)
		if hasAIFields(heartbeat) {
			agent := aiAttributionName(heartbeat)
			addNestedAICostTokens(projectCosts, project, agent, valueOrZero(heartbeat.AIInputTokens), valueOrZero(heartbeat.AIOutputTokens))
		}
		if heartbeat.AISession != "" {
			sessions := projectSessions[project]
			if sessions == nil {
				sessions = map[string]struct{}{}
				projectSessions[project] = sessions
			}
			sessions[heartbeat.AISession] = struct{}{}
		}
	}

	for project, sessions := range projectSessions {
		if stat := projectTotals[project]; stat != nil {
			stat.SessionCount = len(sessions)
		}
	}
	for project, agents := range projectCosts {
		stat := projectTotals[project]
		if stat == nil {
			stat = &AIStat{Name: project}
			projectTotals[project] = stat
		}
		for agent, tokens := range agents {
			stat.EstimatedCostCents += estimateAICostCentsForAgent(agent, tokens.InputTokens, tokens.OutputTokens, costs)
		}
	}

	return sortedProjectAIStats(projectTotals)
}

func addAIStat(totals map[string]*AIStat, name string, heartbeat Heartbeat) {
	stat := totals[name]
	if stat == nil {
		stat = &AIStat{Name: name}
		totals[name] = stat
	}
	stat.AILineChanges += valueOrZero(heartbeat.AILineChanges)
	stat.HumanLineChanges += valueOrZero(heartbeat.HumanLineChanges)
	stat.AIInputTokens += valueOrZero(heartbeat.AIInputTokens)
	stat.AIOutputTokens += valueOrZero(heartbeat.AIOutputTokens)
	stat.AIPromptLength += valueOrZero(heartbeat.AIPromptLength)
}

func aiAttributionName(heartbeat Heartbeat) string {
	for _, value := range []string{heartbeat.AIModel, heartbeat.AIProvider, heartbeat.AIAgent, heartbeat.AISubscriptionPlan} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "Unknown"
}

func addSession(totals map[string]map[string]struct{}, name, session string) {
	if session == "" {
		return
	}
	sessions := totals[name]
	if sessions == nil {
		sessions = map[string]struct{}{}
		totals[name] = sessions
	}
	sessions[session] = struct{}{}
}

func sortedProjectAIStats(totals map[string]*AIStat) []AIStat {
	rows := make([]AIStat, 0, len(totals))
	for _, stat := range totals {
		rows = append(rows, *stat)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AISeconds == rows[j].AISeconds {
			if rows[i].AILineChanges == rows[j].AILineChanges {
				return rows[i].Name < rows[j].Name
			}
			return rows[i].AILineChanges > rows[j].AILineChanges
		}
		return rows[i].AISeconds > rows[j].AISeconds
	})
	return rows
}

func sortedAIStats(totals map[string]*AIStat) []AIStat {
	rows := make([]AIStat, 0, len(totals))
	for _, stat := range totals {
		rows = append(rows, *stat)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AILineChanges == rows[j].AILineChanges {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].AILineChanges > rows[j].AILineChanges
	})
	return rows
}

func sortedAICostPeriods(totals map[string]*aiCostPeriodTokenTotals, costs map[string]AICostRate) []AICostPeriod {
	rows := make([]AICostPeriod, 0, len(totals))
	for _, period := range totals {
		rows = append(rows, AICostPeriod{
			Agent:        period.Agent,
			DailyCents:   estimateAICostCentsForAgent(period.Agent, period.DailyInput, period.DailyOutput, costs),
			WeeklyCents:  estimateAICostCentsForAgent(period.Agent, period.WeeklyInput, period.WeeklyOutput, costs),
			MonthlyCents: estimateAICostCentsForAgent(period.Agent, period.MonthlyInput, period.MonthlyOutput, costs),
			TotalCents:   estimateAICostCentsForAgent(period.Agent, period.TotalInput, period.TotalOutput, costs),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalCents == rows[j].TotalCents {
			return rows[i].Agent < rows[j].Agent
		}
		return rows[i].TotalCents > rows[j].TotalCents
	})
	return rows
}

func orderedDayAIStats(totals map[string]*AIStat, start time.Time, days int) []AIStat {
	rows := make([]AIStat, 0, days)
	for i := 0; i < days; i++ {
		name := start.AddDate(0, 0, i).Format("2006-01-02")
		if stat := totals[name]; stat != nil {
			rows = append(rows, *stat)
		} else {
			rows = append(rows, AIStat{Name: name})
		}
	}
	return rows
}

func hasAIFields(heartbeat Heartbeat) bool {
	if strings.EqualFold(strings.TrimSpace(heartbeat.Category), "ai coding") {
		return true
	}
	return heartbeat.AILineChanges != nil || heartbeat.HumanLineChanges != nil || heartbeat.AIInputTokens != nil ||
		heartbeat.AIOutputTokens != nil || heartbeat.AIPromptLength != nil || heartbeat.AISession != ""
}

func applyWakaTimeAIAliases(metrics *AIMetrics, promptEventsBySession, promptLengthTotalsBySession map[string]int) {
	metrics.AIAdditions = metrics.AILineChanges
	metrics.AIDeletions = 0
	metrics.HumanAdditions = metrics.HumanLineChanges
	metrics.HumanDeletions = 0
	metrics.AILineChangesTotal = metrics.AILineChanges
	metrics.AIPromptLengthAvg = metrics.AveragePromptLength
	metrics.AIPromptLengthSum = metrics.AIPromptLength
	metrics.AIPromptEventsTotal = metrics.PromptCount

	sessionCount := len(promptEventsBySession)
	if sessionCount == 0 {
		return
	}
	eventCounts := make([]int, 0, sessionCount)
	lengthTotals := make([]int, 0, sessionCount)
	for session, events := range promptEventsBySession {
		eventCounts = append(eventCounts, events)
		lengthTotals = append(lengthTotals, promptLengthTotalsBySession[session])
	}
	metrics.AIPromptEventsAvgPerSession = sumInts(eventCounts) / sessionCount
	metrics.AIPromptEventsMedianPerSession = medianInt(eventCounts)
	metrics.AIPromptLengthAvgPerSession = sumInts(lengthTotals) / sessionCount
	metrics.AIPromptLengthMedianPerSession = medianInt(lengthTotals)
}

func applyWakaTimeAgentAliases(metrics *AIMetrics) {
	metrics.AIAgentLineChanges = map[string]int{}
	metrics.AIAgentCosts = map[string]float64{}
	metrics.AIAgentBreakdown = make([]AIAgentBreakdown, 0, len(metrics.Agents))
	for _, agent := range metrics.Agents {
		cost := float64(agent.EstimatedCostCents) / 100
		metrics.AIAgentLineChanges[agent.Name] = agent.AILineChanges
		metrics.AIAgentCosts[agent.Name] = cost
		metrics.AIAgentTotalCost += cost
		metrics.AIAgentBreakdown = append(metrics.AIAgentBreakdown, AIAgentBreakdown{
			Name:  agent.Name,
			Lines: agent.AILineChanges,
			Cost:  cost,
		})
	}
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func estimateAICostCentsForAgent(agent string, inputTokens, outputTokens int, costs map[string]AICostRate) int {
	rate, ok := costs[agent]
	if !ok {
		rate = AICostRate{InputCostPerMillionCents: 3, OutputCostPerMillionCents: 12}
	}
	return (inputTokens*rate.InputCostPerMillionCents + outputTokens*rate.OutputCostPerMillionCents) / 1_000_000
}

func sumDurations(durations []Duration) int {
	total := 0
	for _, duration := range durations {
		total += duration.DurationSeconds
	}
	return total
}

func daysBetween(start, end time.Time) int {
	days := int(end.Sub(start).Hours() / 24)
	if days < 1 {
		return 1
	}
	return days
}

func beginningOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func HumanDuration(seconds int) string {
	if seconds <= 0 {
		return "0 secs"
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("%d hrs %d mins", hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d hrs", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d mins", minutes)
	}
	return fmt.Sprintf("%d secs", seconds)
}
