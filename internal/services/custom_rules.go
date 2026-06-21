package services

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

func ValidateCustomRules(rules []CustomRule) error {
	for _, rule := range rules {
		if rule.Action != "change" && rule.Action != "delete" {
			return errors.New("custom rule action must be change or delete")
		}
		if strings.TrimSpace(rule.Source) == "" || strings.TrimSpace(rule.Operation) == "" || strings.TrimSpace(rule.SourceValue) == "" {
			return errors.New("custom rule source, operation, and source_value are required")
		}
		if !validCustomRuleField(rule.Source) {
			return errors.New("custom rule source is unsupported")
		}
		operation := NormalizeCustomRuleOperation(rule.Operation)
		if !validCustomRuleOperations[operation] {
			return errors.New("custom rule operation must be equals, contains, starts_with, ends_with, or regex")
		}
		if operation == "regex" || operation == "matches" {
			if _, err := regexp.Compile(rule.SourceValue); err != nil {
				return errors.New("custom rule regex is invalid")
			}
		}
		if rule.Action == "change" {
			if len(rule.Destinations) == 0 {
				return errors.New("change custom rules require destinations")
			}
			for _, destination := range rule.Destinations {
				if strings.TrimSpace(destination.Destination) == "" || strings.TrimSpace(destination.DestinationValue) == "" {
					return errors.New("change custom rules require destination and destination_value")
				}
				if !validCustomRuleField(destination.Destination) {
					return errors.New("custom rule destination is unsupported")
				}
			}
		}
	}
	return nil
}

var validCustomRuleOperations = map[string]bool{
	"contains":    true,
	"ends_with":   true,
	"equals":      true,
	"matches":     true,
	"regex":       true,
	"starts_with": true,
}

var validCustomRuleFields = map[string]bool{
	"branch":           true,
	"category":         true,
	"editor":           true,
	"entity":           true,
	"language":         true,
	"operating_system": true,
	"project":          true,
	"type":             true,
}

func validCustomRuleField(value string) bool {
	return validCustomRuleFields[normalizeField(value)]
}

func ApplyCustomRules(heartbeat Heartbeat, rules []CustomRule) (Heartbeat, bool) {
	ordered := append([]CustomRule(nil), rules...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Priority < ordered[j].Priority
	})
	for _, rule := range ordered {
		if !ruleMatches(heartbeat, rule) {
			continue
		}
		if rule.Action == "delete" {
			return heartbeat, true
		}
		if rule.Action == "change" {
			for _, destination := range rule.Destinations {
				heartbeat = setHeartbeatField(heartbeat, destination.Destination, destination.DestinationValue)
			}
		}
	}
	return heartbeat, false
}

func ruleMatches(heartbeat Heartbeat, rule CustomRule) bool {
	value := heartbeatField(heartbeat, rule.Source)
	needle := rule.SourceValue
	switch NormalizeCustomRuleOperation(rule.Operation) {
	case "equals":
		return value == needle
	case "contains":
		return strings.Contains(value, needle)
	case "starts_with":
		return strings.HasPrefix(value, needle)
	case "ends_with":
		return strings.HasSuffix(value, needle)
	case "regex", "matches":
		matched, err := regexp.MatchString(needle, value)
		return err == nil && matched
	default:
		return false
	}
}

func heartbeatField(heartbeat Heartbeat, field string) string {
	switch normalizeField(field) {
	case "entity":
		return heartbeat.Entity
	case "type":
		return heartbeat.Type
	case "category":
		return heartbeat.Category
	case "project":
		return heartbeat.Project
	case "branch":
		return heartbeat.Branch
	case "language":
		return heartbeat.Language
	case "editor":
		return heartbeat.Editor
	case "operating_system":
		return heartbeat.OperatingSystem
	default:
		return ""
	}
}

func setHeartbeatField(heartbeat Heartbeat, field, value string) Heartbeat {
	switch normalizeField(field) {
	case "entity":
		heartbeat.Entity = value
	case "type":
		heartbeat.Type = value
	case "category":
		heartbeat.Category = value
	case "project":
		heartbeat.Project = value
	case "branch":
		heartbeat.Branch = value
	case "language":
		heartbeat.Language = value
	case "editor":
		heartbeat.Editor = value
	case "operating_system":
		heartbeat.OperatingSystem = value
	}
	return heartbeat
}

func NormalizeCustomRuleOperation(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func normalizeField(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
