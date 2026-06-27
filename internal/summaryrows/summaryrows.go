package summaryrows

import (
	"time"

	"github.com/keithah/stint/internal/services"
)

type Fields struct {
	Projects         bool
	Languages        bool
	Categories       bool
	Dependencies     bool
	Editors          bool
	Machines         bool
	OperatingSystems bool
}

func AllFields() Fields {
	return Fields{
		Projects:         true,
		Languages:        true,
		Categories:       true,
		Dependencies:     true,
		Editors:          true,
		Machines:         true,
		OperatingSystems: true,
	}
}

func (f Fields) Any() bool {
	return f.Projects || f.Languages || f.Categories || f.Dependencies || f.Editors || f.Machines || f.OperatingSystems
}

func RowsForRange(heartbeats []services.Heartbeat, external []services.ExternalDuration, startDate, endDate time.Time, timeout time.Duration, fields Fields) []map[string]any {
	heartbeatsByDay := bucketHeartbeatsByDay(heartbeats)
	externalByDay := bucketExternalDurationsByDay(external, startDate, endDate)
	data := []map[string]any{}
	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		next := day.AddDate(0, 0, 1)
		stats, _, _ := services.ComputeStatsForRangeWithExternalDurations(heartbeatsByDay[day], externalByDay[day], day.Add(12*time.Hour), timeout, "last_7_days")
		row := map[string]any{
			"range": map[string]string{
				"date":  day.Format("2006-01-02"),
				"start": day.Format(time.RFC3339),
				"end":   next.Format(time.RFC3339),
			},
			"grand_total": map[string]any{"total_seconds": stats.TotalSeconds, "text": services.HumanDuration(stats.TotalSeconds)},
		}
		if fields.Projects {
			row["projects"] = stats.Projects
		}
		if fields.Languages {
			row["languages"] = stats.Languages
		}
		if fields.Categories {
			row["categories"] = stats.Categories
		}
		if fields.Dependencies {
			row["dependencies"] = stats.Dependencies
		}
		if fields.Editors {
			row["editors"] = stats.Editors
		}
		if fields.Machines {
			row["machines"] = stats.Machines
		}
		if fields.OperatingSystems {
			row["operating_systems"] = stats.OperatingSystems
		}
		data = append(data, row)
	}
	return data
}

func DateRange(heartbeats []services.Heartbeat, external []services.ExternalDuration, now time.Time) (time.Time, time.Time) {
	start := UTCDate(now)
	end := start
	expand := func(t time.Time) {
		day := UTCDate(t)
		if day.Before(start) {
			start = day
		}
		if day.After(end) {
			end = day
		}
	}
	for _, heartbeat := range heartbeats {
		if heartbeat.Time > 0 {
			expand(time.Unix(int64(heartbeat.Time), 0).UTC())
		}
	}
	for _, duration := range external {
		if duration.StartTime > 0 {
			expand(time.Unix(int64(duration.StartTime), 0).UTC())
		}
		if duration.EndTime > 0 {
			expand(time.Unix(int64(duration.EndTime), 0).UTC())
		}
	}
	return start, end
}

func UTCDate(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func bucketHeartbeatsByDay(heartbeats []services.Heartbeat) map[time.Time][]services.Heartbeat {
	byDay := make(map[time.Time][]services.Heartbeat)
	for _, heartbeat := range heartbeats {
		if heartbeat.Time <= 0 {
			continue
		}
		day := UTCDate(time.Unix(int64(heartbeat.Time), 0).UTC())
		byDay[day] = append(byDay[day], heartbeat)
	}
	return byDay
}

func bucketExternalDurationsByDay(external []services.ExternalDuration, startDate, endDate time.Time) map[time.Time][]services.ExternalDuration {
	byDay := make(map[time.Time][]services.ExternalDuration)
	for _, duration := range external {
		started := time.Unix(int64(duration.StartTime), 0).UTC()
		ended := time.Unix(int64(duration.EndTime), 0).UTC()
		if !ended.After(started) {
			continue
		}
		first := UTCDate(started)
		if first.Before(startDate) {
			first = startDate
		}
		last := UTCDate(ended.Add(-time.Nanosecond))
		if last.After(endDate) {
			last = endDate
		}
		for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
			byDay[day] = append(byDay[day], duration)
		}
	}
	return byDay
}
