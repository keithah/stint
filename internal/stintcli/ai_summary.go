package stintcli

import (
	"path/filepath"
	"sort"
	"strings"
)

func aiSummaryHeartbeats(summary aiTranscriptSummary, opts Options) []Heartbeat {
	sessionID := first(summary.SessionID, "unknown")
	project, branch := aiProject(summary.CWD, opts)
	baseTime := float64(summary.LastActivity.UnixNano()) / 1e9
	app := Heartbeat{
		AIAgent:            first(summary.Agent, strings.ToLower(summary.Source)),
		AIAgentVersion:     summary.AgentVersion,
		AISession:          sessionID,
		AIInputTokens:      intPointer(summary.InputTokens),
		AIModel:            summary.Model,
		AIOutputTokens:     intPointer(summary.OutputTokens),
		AIProvider:         summary.Provider,
		AIPromptLength:     intPointer(summary.PromptLength),
		AISubscriptionPlan: summary.SubscriptionPlan,
		Branch:             branch,
		Category:           aiCodingCategory,
		Entity:             summary.Source + " " + sessionID,
		EntityType:         "app",
		MachineName:        machineName(opts.Hostname),
		Plugin:             first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript"),
		Project:            first(opts.Project, project, opts.AlternateProject),
		Time:               baseTime,
		UserAgent:          userAgent(first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript")),
	}
	heartbeats := []Heartbeat{app}
	if len(summary.FileEvents) > 0 {
		for i, event := range summary.FileEvents {
			if i >= 20 || !fileExists(event.Path) {
				continue
			}
			fileProject, fileBranch, _ := detectProject(event.Path, opts)
			heartbeats = append(heartbeats, Heartbeat{
				AIAgent:            first(summary.Agent, strings.ToLower(summary.Source)),
				AIAgentVersion:     summary.AgentVersion,
				AILineChanges:      event.LineChanges,
				AISession:          sessionID,
				AIModel:            summary.Model,
				AIProvider:         summary.Provider,
				AISubscriptionPlan: summary.SubscriptionPlan,
				Branch:             first(opts.Branch, fileBranch, branch, opts.AlternateBranch),
				Category:           aiCodingCategory,
				Entity:             event.Path,
				EntityType:         "file",
				IsWrite:            event.IsWrite,
				Language:           detectLanguage(event.Path),
				MachineName:        machineName(opts.Hostname),
				Plugin:             first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript"),
				Project:            first(opts.Project, fileProject, project, opts.AlternateProject),
				Time:               baseTime + float64(i+1)/1000,
				UserAgent:          userAgent(first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript")),
			})
		}
		return heartbeats
	}
	files := sortedAIPaths(summary.Files)
	for i, path := range files {
		if i >= 20 {
			break
		}
		if !fileExists(path) {
			continue
		}
		fileProject, fileBranch, _ := detectProject(path, opts)
		var aiLineChanges *int
		if lineChanges, ok := summary.LineChanges[path]; ok {
			aiLineChanges = intPointerAllowZero(lineChanges)
		}
		isWrite, ok := summary.FileWrites[path]
		if !ok {
			isWrite = true
		}
		heartbeats = append(heartbeats, Heartbeat{
			AIAgent:            first(summary.Agent, strings.ToLower(summary.Source)),
			AIAgentVersion:     summary.AgentVersion,
			AILineChanges:      aiLineChanges,
			AISession:          sessionID,
			AIModel:            summary.Model,
			AIProvider:         summary.Provider,
			AISubscriptionPlan: summary.SubscriptionPlan,
			Branch:             first(opts.Branch, fileBranch, branch, opts.AlternateBranch),
			Category:           aiCodingCategory,
			Entity:             path,
			EntityType:         "file",
			IsWrite:            isWrite,
			Language:           detectLanguage(path),
			MachineName:        machineName(opts.Hostname),
			Plugin:             first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript"),
			Project:            first(opts.Project, fileProject, project, opts.AlternateProject),
			Time:               baseTime + float64(i+1)/1000,
			UserAgent:          userAgent(first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript")),
		})
	}
	return heartbeats
}

func intPointerAllowZero(value int) *int {
	return &value
}

func aiProject(cwd string, opts Options) (string, string) {
	if cwd == "" {
		return "", ""
	}
	root := findProjectRoot(cwd)
	if root == "" {
		root = cwd
	}
	project, branch, _ := detectProject(filepath.Join(root, ".ai-session"), Options{
		AlternateBranch:  opts.AlternateBranch,
		AlternateProject: opts.AlternateProject,
		Branch:           opts.Branch,
		Config:           opts.Config,
		EntityType:       "app",
		Project:          opts.Project,
		ProjectFolder:    root,
	})
	return project, branch
}

func sortedAIPaths(paths map[string]bool) []string {
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}
