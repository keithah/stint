package services

import "testing"

func TestValidateCustomRulesRejectsInvalidRules(t *testing.T) {
	valid := CustomRule{
		Action:      "change",
		Source:      "entity",
		Operation:   "contains",
		SourceValue: "legacy",
		Priority:    1,
		Destinations: []CustomRuleDestination{
			{Destination: "project", DestinationValue: "modernized"},
		},
	}

	tests := []struct {
		name string
		rule CustomRule
	}{
		{name: "invalid action", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Action = "move" })},
		{name: "missing source", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Source = "" })},
		{name: "unsupported source", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Source = "workspace" })},
		{name: "missing operation", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Operation = "" })},
		{name: "missing source value", rule: withCustomRule(valid, func(rule *CustomRule) { rule.SourceValue = "" })},
		{name: "invalid operation", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Operation = "glob" })},
		{name: "invalid regex", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Operation = "regex"; rule.SourceValue = "[" })},
		{name: "missing change destination", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Destinations = nil })},
		{name: "blank destination", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Destinations[0].Destination = "" })},
		{name: "blank destination value", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Destinations[0].DestinationValue = "" })},
		{name: "unsupported destination", rule: withCustomRule(valid, func(rule *CustomRule) { rule.Destinations[0].Destination = "workspace" })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateCustomRules([]CustomRule{tt.rule}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateCustomRulesAcceptsDeleteAndRegexAliases(t *testing.T) {
	err := ValidateCustomRules([]CustomRule{
		{
			Action:      "delete",
			Source:      "entity",
			Operation:   "matches",
			SourceValue: `vendor/.+\.go$`,
			Priority:    1,
		},
		{
			Action:      "change",
			Source:      "branch",
			Operation:   "equals",
			SourceValue: "main",
			Priority:    2,
			Destinations: []CustomRuleDestination{
				{Destination: "project", DestinationValue: "mainline"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected valid delete regex alias rule, got %v", err)
	}
}

func TestApplyCustomRulesChangesHeartbeatDestination(t *testing.T) {
	heartbeat := Heartbeat{Entity: "/work/legacy/main.go", Project: "old", Language: "Go"}
	rules := []CustomRule{{
		Action:      "change",
		Source:      "entity",
		Operation:   "contains",
		SourceValue: "legacy",
		Priority:    10,
		Destinations: []CustomRuleDestination{
			{Destination: "project", DestinationValue: "modernized"},
			{Destination: "language", DestinationValue: "Go"},
		},
	}}

	got, deleted := ApplyCustomRules(heartbeat, rules)

	if deleted {
		t.Fatal("expected heartbeat to be kept")
	}
	if got.Project != "modernized" {
		t.Fatalf("expected project modernized, got %q", got.Project)
	}
	if got.Language != "Go" {
		t.Fatalf("expected language Go, got %q", got.Language)
	}
}

func TestApplyCustomRulesDeletesHeartbeat(t *testing.T) {
	heartbeat := Heartbeat{Entity: "https://example.com", Type: "url", Project: "browser"}
	rules := []CustomRule{{
		Action:      "delete",
		Source:      "type",
		Operation:   "equals",
		SourceValue: "url",
		Priority:    1,
	}}

	_, deleted := ApplyCustomRules(heartbeat, rules)

	if !deleted {
		t.Fatal("expected heartbeat to be deleted")
	}
}

func TestApplyCustomRulesMatchesRegex(t *testing.T) {
	heartbeat := Heartbeat{Entity: "/work/client-42/generated/main.go", Project: "old"}
	rules := []CustomRule{{
		Action:      "change",
		Source:      "entity",
		Operation:   "regex",
		SourceValue: `client-\d+/generated`,
		Priority:    1,
		Destinations: []CustomRuleDestination{
			{Destination: "project", DestinationValue: "generated"},
		},
	}}

	got, deleted := ApplyCustomRules(heartbeat, rules)

	if deleted {
		t.Fatal("expected heartbeat to be kept")
	}
	if got.Project != "generated" {
		t.Fatalf("expected regex rule to rewrite project, got %q", got.Project)
	}
}

func withCustomRule(input CustomRule, mutate func(*CustomRule)) CustomRule {
	input.Destinations = append([]CustomRuleDestination(nil), input.Destinations...)
	mutate(&input)
	return input
}
