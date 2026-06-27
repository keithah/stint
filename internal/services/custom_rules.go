package services

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

const (
	maxCustomRules          = 100
	maxCustomRuleRegexBytes = 512
	maxCustomRuleMatchBytes = 4096
)

func ValidateCustomRules(rules []CustomRule) error {
	if len(rules) > maxCustomRules {
		return errors.New("custom rule limit is 100")
	}
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
			if len(rule.SourceValue) > maxCustomRuleRegexBytes {
				return errors.New("custom rule regex is too long")
			}
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
	prepared, err := PrepareCustomRules(rules)
	if err != nil {
		return heartbeat, false
	}
	return ApplyPreparedCustomRules(heartbeat, prepared)
}

type PreparedCustomRules []preparedCustomRule

type preparedCustomRule struct {
	rule      CustomRule
	operation string
	regex     *regexp.Regexp
}

func PrepareCustomRules(rules []CustomRule) (PreparedCustomRules, error) {
	ordered := append([]CustomRule(nil), rules...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Priority < ordered[j].Priority
	})
	prepared := make(PreparedCustomRules, 0, len(ordered))
	for _, rule := range ordered {
		operation := NormalizeCustomRuleOperation(rule.Operation)
		item := preparedCustomRule{rule: rule, operation: operation}
		if operation == "regex" || operation == "matches" {
			compiled, err := regexp.Compile(rule.SourceValue)
			if err != nil {
				return nil, err
			}
			item.regex = compiled
		}
		prepared = append(prepared, item)
	}
	return prepared, nil
}

func ApplyPreparedCustomRules(heartbeat Heartbeat, rules PreparedCustomRules) (Heartbeat, bool) {
	for _, rule := range rules {
		if !preparedRuleMatches(heartbeat, rule) {
			continue
		}
		if rule.rule.Action == "delete" {
			return heartbeat, true
		}
		if rule.rule.Action == "change" {
			for _, destination := range rule.rule.Destinations {
				heartbeat = setHeartbeatField(heartbeat, destination.Destination, destination.DestinationValue)
			}
		}
	}
	return heartbeat, false
}

func preparedRuleMatches(heartbeat Heartbeat, rule preparedCustomRule) bool {
	source := rule.rule.Source
	needle := rule.rule.SourceValue
	value := heartbeatField(heartbeat, source)
	switch rule.operation {
	case "equals":
		return value == needle
	case "contains":
		return strings.Contains(value, needle)
	case "starts_with":
		return strings.HasPrefix(value, needle)
	case "ends_with":
		return strings.HasSuffix(value, needle)
	case "regex", "matches":
		if len(value) > maxCustomRuleMatchBytes {
			value = value[:maxCustomRuleMatchBytes]
		}
		return rule.regex != nil && rule.regex.MatchString(value)
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
