package services

import (
	"errors"
	"strings"
	"time"
)

func NormalizeLeaderboardInput(name, rangeName string) (string, string) {
	name = strings.TrimSpace(name)
	rangeName = strings.TrimSpace(rangeName)
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	return name, rangeName
}

func ValidateLeaderboardInput(name string, rangeName string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("leaderboard name is required")
	}
	rangeName = strings.TrimSpace(rangeName)
	if rangeName == "" {
		return nil
	}
	if rangeName == "all_time" {
		return nil
	}
	if _, err := WindowForRange(time.Now(), rangeName); err != nil {
		return errors.New("time_range must be a supported stats range")
	}
	return nil
}
