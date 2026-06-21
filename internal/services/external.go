package services

import (
	"errors"
	"sort"
	"strings"
)

func ValidateExternalDuration(input ExternalDuration) error {
	if strings.TrimSpace(input.ExternalID) == "" {
		return errors.New("external_id is required")
	}
	if strings.TrimSpace(input.Provider) == "" {
		return errors.New("provider is required")
	}
	if strings.TrimSpace(input.Entity) == "" {
		return errors.New("entity is required")
	}
	if strings.TrimSpace(input.Type) == "" {
		return errors.New("type is required")
	}
	if input.StartTime <= 0 || input.EndTime <= 0 || input.EndTime <= input.StartTime {
		return errors.New("valid start_time and end_time are required")
	}
	return nil
}

func ExternalDurationTotals(external []ExternalDuration, sliceBy string) []SliceTotal {
	totals := map[string]int{}
	for _, duration := range external {
		name := externalSliceName(duration, sliceBy)
		seconds := int(duration.EndTime - duration.StartTime)
		if seconds < 0 {
			seconds = 0
		}
		totals[name] += seconds
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

func RankLeaderboardEntries(entries []LeaderboardEntry) []LeaderboardEntry {
	rows := append([]LeaderboardEntry(nil), entries...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalSeconds == rows[j].TotalSeconds {
			return rows[i].Username < rows[j].Username
		}
		return rows[i].TotalSeconds > rows[j].TotalSeconds
	})
	for i := range rows {
		rows[i].Rank = i + 1
		rows[i].Text = HumanDuration(rows[i].TotalSeconds)
	}
	return rows
}

func externalSliceName(duration ExternalDuration, sliceBy string) string {
	switch sliceBy {
	case "language":
		if duration.Language != "" {
			return duration.Language
		}
	case "category":
		if duration.Category != "" {
			return duration.Category
		}
	case "branch":
		if duration.Branch != "" {
			return duration.Branch
		}
	default:
		if duration.Project != "" {
			return duration.Project
		}
	}
	return "Unknown"
}
