package stintcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type todaySummary struct {
	Data            todayData `json:"data"`
	HasTeamFeatures bool      `json:"has_team_features"`
}

type todayData struct {
	Categories []todayCounter `json:"categories"`
	GrandTotal todayCounter   `json:"grand_total"`
}

type todayCounter struct {
	Hours int    `json:"hours"`
	Name  string `json:"name"`
	Text  string `json:"text"`
}

type todayGoal struct {
	Data todayGoalData `json:"data"`
}

type todayGoalData struct {
	ChartData []todayGoalChartData `json:"chart_data"`
}

type todayGoalChartData struct {
	ActualSecondsText string `json:"actual_seconds_text"`
}

func writeTodayOutput(stdout io.Writer, opts Options, body []byte) error {
	format := strings.TrimSpace(opts.Output)
	switch format {
	case "raw-json":
		_, err := stdout.Write(body)
		if err == nil && !strings.HasSuffix(string(body), "\n") {
			_, err = fmt.Fprintln(stdout)
		}
		return err
	case "json":
		var summary todaySummary
		if err := json.Unmarshal(body, &summary); err != nil {
			return err
		}
		payload := map[string]any{
			"text":              todayOutputText(summary, opts),
			"has_team_features": summary.HasTeamFeatures,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(encoded))
		return err
	case "", "text":
		var summary todaySummary
		if err := json.Unmarshal(body, &summary); err != nil {
			return err
		}
		_, err := fmt.Fprintln(stdout, todayOutputText(summary, opts))
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func todayOutputText(summary todaySummary, opts Options) string {
	if !opts.TodayStatusBarEnabled {
		return ""
	}
	if !opts.TodayCodingActivity {
		return ""
	}
	return todayText(summary, opts.TodayHideCategories, opts.TodayHideMinutes, opts.TodayMaxCategories)
}

func writeTodayGoalOutput(stdout io.Writer, opts Options, body []byte) error {
	format := strings.TrimSpace(opts.Output)
	switch format {
	case "raw-json":
		_, err := stdout.Write(body)
		if err == nil && !strings.HasSuffix(string(body), "\n") {
			_, err = fmt.Fprintln(stdout)
		}
		return err
	case "", "text", "json":
		var goal todayGoal
		if err := json.Unmarshal(body, &goal); err != nil {
			return err
		}
		if len(goal.Data.ChartData) == 0 {
			return errors.New("no chart data found for the current day")
		}
		_, err := fmt.Fprintln(stdout, goal.Data.ChartData[len(goal.Data.ChartData)-1].ActualSecondsText)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func todayText(summary todaySummary, hideCategories, hideMinutes bool, maxCategories int) string {
	if len(summary.Data.Categories) < 2 || hideCategories {
		return durationText(summary.Data.GrandTotal.Hours, summary.Data.GrandTotal.Text, hideMinutes)
	}
	categories := summary.Data.Categories
	if maxCategories > 0 && len(categories) > maxCategories {
		categories = categories[:maxCategories]
	}
	parts := make([]string, 0, len(categories))
	for _, category := range categories {
		parts = append(parts, strings.TrimSpace(durationText(category.Hours, category.Text, hideMinutes)+" "+category.Name))
	}
	out := strings.Join(parts, ", ")
	if maxCategories > 1 && len(summary.Data.Categories) > maxCategories {
		out += "..."
	}
	return out
}

func durationText(hours int, text string, hideMinutes bool) string {
	if !hideMinutes || hours == 0 {
		return text
	}
	if hours == 1 {
		return "1 hr"
	}
	return fmt.Sprintf("%d hrs", hours)
}
