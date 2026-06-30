package stintcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func sendHeartbeat(stdin io.Reader, stdout io.Writer, opts Options) error {
	hb, err := BuildHeartbeat(opts)
	if err != nil {
		if errors.Is(err, errHeartbeatFiltered) {
			return nil
		}
		return err
	}
	heartbeats := []Heartbeat{hb}
	if opts.ExtraHeartbeats {
		extra, err := readExtraHeartbeats(stdin, opts)
		if err != nil {
			return err
		}
		heartbeats = append(heartbeats, extra...)
	}
	var lastAIActivity time.Time
	if !opts.SyncAIDisabled {
		aiHeartbeats, lastActivity, err := collectAIHeartbeats(opts)
		if err == nil && len(aiHeartbeats) > 0 {
			preserveHumanAttributesOnAI(heartbeats, aiHeartbeats)
			heartbeats = mergeHumanHeartbeatsWithAI(heartbeats, aiHeartbeats)
			heartbeats = append(heartbeats, aiHeartbeats...)
			lastAIActivity = lastActivity
		}
	}
	if err := sendHeartbeats(stdout, opts, heartbeats, hb.Entity, true, true); err != nil {
		return err
	}
	return recordAISyncAfter(opts, lastAIActivity)
}

func preserveHumanAttributesOnAI(human, ai []Heartbeat) {
	if len(human) == 0 || len(ai) == 0 {
		return
	}
	byEntity := make(map[string][]Heartbeat, len(human))
	for _, hb := range human {
		byEntity[hb.Entity] = append(byEntity[hb.Entity], hb)
	}
	for i := range ai {
		aiHB := &ai[i]
		for _, humanHB := range byEntity[aiHB.Entity] {
			preserveHeartbeatAttributes(aiHB, humanHB)
		}
	}
}

func preserveHeartbeatAttributes(dst *Heartbeat, src Heartbeat) {
	if dst.Project == "" {
		dst.Project = src.Project
	}
	if dst.Branch == "" {
		dst.Branch = src.Branch
	}
	if dst.Language == "" {
		dst.Language = src.Language
	}
	if src.Lines != nil && *src.Lines > 0 {
		dst.Lines = src.Lines
	}
	if dst.ProjectRootCount == nil && src.ProjectRootCount != nil {
		dst.ProjectRootCount = src.ProjectRootCount
	}
}

func mergeHumanHeartbeatsWithAI(human, ai []Heartbeat) []Heartbeat {
	if len(human) == 0 || len(ai) == 0 {
		return human
	}
	minTime, maxTime := ai[0].Time, ai[0].Time
	aiEntityTimes := map[string][]float64{}
	for _, hb := range ai {
		aiEntityTimes[hb.Entity] = append(aiEntityTimes[hb.Entity], hb.Time)
	}
	for entity := range aiEntityTimes {
		sort.Float64s(aiEntityTimes[entity])
	}
	for _, hb := range ai[1:] {
		if hb.Time < minTime {
			minTime = hb.Time
		}
		if hb.Time > maxTime {
			maxTime = hb.Time
		}
	}
	var firstHumanEdit *float64
	for i := range human {
		hb := &human[i]
		if hb.Time > maxTime+1 && (firstHumanEdit == nil || hb.Time < *firstHumanEdit) && hb.HumanLineChanges != nil && *hb.HumanLineChanges != 0 {
			firstHumanEdit = &hb.Time
		}
	}
	kept := make([]Heartbeat, 0, len(human))
	for i := range human {
		hb := &human[i]
		if sameEntityHeartbeatWithinWindow(*hb, aiEntityTimes, 5) && (firstHumanEdit == nil || hb.Time < *firstHumanEdit) {
			continue
		}
		inRange := func(minutes float64) bool {
			window := minutes * 60
			return hb.Time > minTime-window && hb.Time < maxTime+window
		}
		if (inRange(2) && (hb.HumanLineChanges == nil || *hb.HumanLineChanges == 0)) ||
			((firstHumanEdit == nil || hb.Time < *firstHumanEdit) && inRange(30)) {
			hb.Category = aiCodingCategory
		}
		kept = append(kept, *hb)
	}
	return kept
}

func sameEntityHeartbeatWithinWindow(hb Heartbeat, entityTimes map[string][]float64, windowSeconds float64) bool {
	times := entityTimes[hb.Entity]
	if len(times) == 0 {
		return false
	}
	lower := hb.Time - windowSeconds
	upper := hb.Time + windowSeconds
	index := sort.SearchFloat64s(times, lower)
	return index < len(times) && times[index] <= upper
}

func sendHeartbeats(stdout io.Writer, opts Options, heartbeats []Heartbeat, _ string, syncQueued, writeResponse bool) error {
	if len(heartbeats) == 0 {
		return nil
	}
	if opts.shouldBackoff() {
		if opts.DisableOffline {
			return fmt.Errorf("won't send heartbeat due to backoff")
		}
		if err := AppendQueue(opts.QueuePath, heartbeats); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "queued=%d\n", len(heartbeats))
		return nil
	}
	if len(heartbeats) > defaultHeartbeatLimit {
		extraHeartbeats := append([]Heartbeat(nil), heartbeats[defaultHeartbeatLimit:]...)
		heartbeats = heartbeats[:defaultHeartbeatLimit]
		if !opts.DisableOffline {
			if err := AppendQueue(opts.QueuePath, extraHeartbeats); err != nil {
				return err
			}
		}
	}
	if opts.DisableOffline || !opts.shouldQueueForRateLimit() {
		groups, targets, err := heartbeatTargetGroups(opts, heartbeats)
		if err != nil {
			return err
		}
		bodies := make([]json.RawMessage, 0, len(targets))
		for _, target := range targets {
			targetOpts := opts
			targetOpts.APIURL = target.APIURL
			targetOpts.APIKey = target.APIKey
			client, err := NewClient(targetOpts)
			if err != nil {
				return err
			}
			body, err := client.PostJSON(context.Background(), "/users/current/heartbeats.bulk", groups[target])
			if err != nil {
				if opts.DisableOffline {
					return err
				}
				if qerr := AppendQueue(opts.QueuePath, heartbeats); qerr != nil {
					return errors.Join(err, qerr)
				}
				_ = opts.recordBackoffFailure()
				fmt.Fprintf(stdout, "queued=%d\n", len(heartbeats))
				return nil
			}
			bodies = append(bodies, json.RawMessage(body))
		}
		if err := opts.recordLastSent(); err != nil {
			return err
		}
		if err := opts.resetBackoff(); err != nil {
			return err
		}
		body := []byte("{}")
		if len(bodies) == 1 {
			body = bodies[0]
		} else if len(bodies) > 1 {
			body, _ = json.Marshal(map[string]any{"targets": bodies})
		}
		if writeResponse && strings.TrimSpace(opts.Output) != "" {
			if err := writeOutput(stdout, opts.Output, body); err != nil {
				return err
			}
		}
		if syncQueued {
			limit := defaultQueueMaxSync
			if opts.SyncOfflineSet {
				limit = opts.SyncOffline
			}
			return syncOffline(stdout, opts, limit)
		}
		return nil
	}
	if err := AppendQueue(opts.QueuePath, heartbeats); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "queued=%d\n", len(heartbeats))
	return nil
}

type apiTarget struct {
	APIURL string
	APIKey string
}

func heartbeatTargetGroups(opts Options, heartbeats []Heartbeat) (map[apiTarget][]Heartbeat, []apiTarget, error) {
	groups := map[apiTarget][]Heartbeat{}
	seen := map[apiTarget]bool{}
	var orderedTargets []apiTarget
	for _, hb := range heartbeats {
		targets, err := heartbeatTargets(opts, hb.Entity)
		if err != nil {
			return nil, nil, err
		}
		for _, target := range targets {
			if !seen[target] {
				seen[target] = true
				orderedTargets = append(orderedTargets, target)
			}
			groups[target] = append(groups[target], hb)
		}
	}
	return groups, orderedTargets, nil
}

func heartbeatTargets(opts Options, entity string) ([]apiTarget, error) {
	defaultKey := opts.APIKey
	for _, entry := range opts.Config.OrderedSection("project_api_key") {
		re, err := compileWakaPattern(entry.Key)
		if err != nil {
			continue
		}
		if strings.TrimSpace(entry.Value) == "" {
			return nil, fmt.Errorf("invalid api key format for %q", entry.Key)
		}
		if re.MatchString(entity) {
			defaultKey = strings.TrimSpace(entry.Value)
			break
		}
	}
	targets := []apiTarget{{APIURL: opts.APIURL, APIKey: defaultKey}}
	for _, entry := range opts.Config.OrderedSection("api_urls") {
		re, err := compileWakaPattern(entry.Key)
		if err != nil {
			continue
		}
		if !re.MatchString(entity) {
			continue
		}
		apiURL, apiKey, _ := strings.Cut(entry.Value, "|")
		if strings.Contains(entry.Value, "|") && strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("invalid api key format in api_urls for %q", entry.Key)
		}
		targets = append(targets, apiTarget{
			APIURL: first(apiURL, opts.APIURL),
			APIKey: first(apiKey, defaultKey),
		})
	}
	deduped := make([]apiTarget, 0, len(targets))
	seen := map[apiTarget]bool{}
	for _, target := range targets {
		apiURL, err := normalizeAPIURL(target.APIURL)
		if err != nil {
			continue
		}
		target.APIURL = apiURL
		target.APIKey = strings.TrimSpace(target.APIKey)
		if target.APIURL == "" || target.APIKey == "" {
			continue
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		deduped = append(deduped, target)
	}
	if len(deduped) == 0 {
		return nil, fmt.Errorf("api key is required")
	}
	return deduped, nil
}

func readExtraHeartbeats(stdin io.Reader, opts Options) ([]Heartbeat, error) {
	extra, err := decodeExtraHeartbeats(stdin)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, nil
	}
	processed := extra[:0]
	for _, hb := range extra {
		hb, skip, err := processExtraHeartbeat(hb, opts)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		processed = append(processed, hb)
	}
	return processed, nil
}

func decodeExtraHeartbeats(stdin io.Reader) ([]Heartbeat, error) {
	var raw []map[string]json.RawMessage
	if err := json.NewDecoder(stdin).Decode(&raw); err != nil {
		return nil, err
	}
	heartbeats := make([]Heartbeat, 0, len(raw))
	for _, item := range raw {
		hb, err := decodeExtraHeartbeat(item)
		if err != nil {
			return nil, err
		}
		heartbeats = append(heartbeats, hb)
	}
	return heartbeats, nil
}

func decodeExtraHeartbeat(raw map[string]json.RawMessage) (Heartbeat, error) {
	normal := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		switch key {
		case "ai_input_tokens", "ai_line_changes", "ai_output_tokens", "ai_prompt_length", "alternate_branch", "alternate_language", "alternate_project", "cursorpos", "entity_type", "human_line_changes", "is_unsaved_entity", "is_write", "lineno", "lines", "local_file", "time", "timestamp", "user_agent":
			continue
		default:
			normal[key] = value
		}
	}
	data, err := json.Marshal(normal)
	if err != nil {
		return Heartbeat{}, err
	}
	var hb Heartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return Heartbeat{}, err
	}
	if v, ok, err := rawExtraHeartbeatTime(raw); err != nil {
		return Heartbeat{}, err
	} else if ok {
		hb.Time = v
	} else {
		return Heartbeat{}, fmt.Errorf("skipping extra heartbeat, as no valid timestamp was defined")
	}
	if hb.EntityType == "" {
		hb.EntityType = rawString(raw, "entity_type")
	}
	if hb.EntityType != "" && !validEntityType(hb.EntityType) {
		return Heartbeat{}, fmt.Errorf("invalid entity type %q", hb.EntityType)
	}
	if !validCategory(hb.Category) {
		return Heartbeat{}, fmt.Errorf("failed to parse category: invalid category %q", hb.Category)
	}
	hb.AlternateBranch = rawString(raw, "alternate_branch")
	hb.AlternateLanguage = rawString(raw, "alternate_language")
	hb.AlternateProject = rawString(raw, "alternate_project")
	hb.LocalFile = rawString(raw, "local_file")
	if v, ok, err := rawFlexibleBool(raw, "is_unsaved_entity"); err != nil {
		return Heartbeat{}, err
	} else if ok {
		hb.IsUnsavedEntity = v
	}
	if v, ok, err := rawFlexibleBool(raw, "is_write"); err != nil {
		return Heartbeat{}, err
	} else if ok {
		hb.IsWrite = v
		hb.IsWriteSet = true
	}
	assignInt := func(target **int, key string) error {
		v, ok, err := rawFlexibleInt(raw, key)
		if err != nil || !ok {
			return err
		}
		*target = &v
		return nil
	}
	for key, target := range map[string]**int{
		"ai_input_tokens":    &hb.AIInputTokens,
		"ai_line_changes":    &hb.AILineChanges,
		"ai_output_tokens":   &hb.AIOutputTokens,
		"ai_prompt_length":   &hb.AIPromptLength,
		"cursorpos":          &hb.CursorPosition,
		"human_line_changes": &hb.HumanLineChanges,
		"lineno":             &hb.LineNumber,
		"lines":              &hb.Lines,
	} {
		if err := assignInt(target, key); err != nil {
			return Heartbeat{}, err
		}
	}
	return hb, nil
}

func rawString(raw map[string]json.RawMessage, key string) string {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return ""
	}
	return text
}

func rawFlexibleInt(raw map[string]json.RawMessage, key string) (int, bool, error) {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return 0, false, nil
	}
	var number int
	if err := json.Unmarshal(value, &number); err == nil {
		return number, true, nil
	}
	var floatNumber float64
	if err := json.Unmarshal(value, &floatNumber); err == nil {
		return int(floatNumber), true, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return 0, false, err
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, false, err
	}
	return parsed, true, nil
}

func rawFlexibleFloat(raw map[string]json.RawMessage, keys ...string) (float64, bool, error) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || string(value) == "null" {
			continue
		}
		var number float64
		if err := json.Unmarshal(value, &number); err == nil {
			return number, true, nil
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return 0, false, err
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return 0, false, err
		}
		return parsed, true, nil
	}
	return 0, false, nil
}

func rawExtraHeartbeatTime(raw map[string]json.RawMessage) (float64, bool, error) {
	timeValue, timeSet, err := rawFlexibleFloat(raw, "time")
	if err != nil {
		return 0, false, err
	}
	if timeSet && timeValue != 0 {
		return timeValue, true, nil
	}
	timestampValue, timestampSet, err := rawFlexibleFloat(raw, "timestamp")
	if err != nil {
		return 0, false, err
	}
	if timestampSet && timestampValue != 0 {
		return timestampValue, true, nil
	}
	return 0, false, nil
}

func rawFlexibleBool(raw map[string]json.RawMessage, key string) (bool, bool, error) {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return false, false, nil
	}
	var flag bool
	if err := json.Unmarshal(value, &flag); err == nil {
		return flag, true, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return false, false, err
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(text))
	if err != nil {
		return false, false, err
	}
	return parsed, true, nil
}

func processExtraHeartbeat(hb Heartbeat, opts Options) (Heartbeat, bool, error) {
	if hb.Entity == "" {
		return hb, true, nil
	}
	if hb.EntityType == "" {
		hb.EntityType = "file"
	}
	entity, localFile, statsFile, cleanupRemoteStatsFile, remoteEntity, err := resolveStatsFile(hb.Entity, hb.EntityType, hb.LocalFile, hb.IsUnsavedEntity)
	if cleanupRemoteStatsFile != nil {
		defer cleanupRemoteStatsFile()
	}
	if err != nil {
		return hb, false, err
	}
	hb.Entity = entity
	hb.LocalFile = localFile
	if hb.EntityType == "file" && !hb.IsUnsavedEntity {
		if _, err := os.Stat(statsFile); err != nil {
			return hb, true, nil
		}
	}
	if skip, err := excluded(hb.Entity, opts.Include, opts.Exclude); err != nil || skip {
		return hb, skip, err
	}
	project, branch, root := detectProject(statsFile, opts)
	if opts.IncludeOnlyProjectFile && !remoteEntity && (root == "" || !fileExists(filepath.Join(root, ".wakatime-project"))) {
		return hb, true, nil
	}
	if opts.ExcludeUnknownProject && first(hb.Project, project) == "" {
		return hb, true, nil
	}
	if hb.Time == 0 {
		hb.Time = float64(time.Now().UnixNano()) / 1e9
	}
	if hb.Project == "" {
		hb.Project = first(opts.Project, project, hb.AlternateProject, opts.AlternateProject)
	}
	if hb.Branch == "" {
		hb.Branch = first(opts.Branch, branch, hb.AlternateBranch, opts.AlternateBranch)
	}
	if hb.Language == "" && hb.EntityType == "file" {
		hb.Language = first(detectLanguageWithGuess(statsFile, opts.GuessLanguage), detectLanguageWithGuess(hb.Entity, opts.GuessLanguage), hb.AlternateLanguage, opts.AlternateLanguage)
	}
	if hb.MachineName == "" {
		hb.MachineName = machineName(opts.Hostname)
	}
	if hb.UserAgent == "" {
		hb.UserAgent = userAgent(opts.Plugin)
	}
	if hb.Plugin == "" {
		hb.Plugin = opts.Plugin
	}
	if hb.PluginVersion == "" {
		hb.PluginVersion = opts.PluginVersion
	}
	extraCategorySet := hb.Category != ""
	if hb.Category == "" {
		hb.Category = opts.Category
	}
	hb.Category = normalizeCategoryForSend(hb.Category, (extraCategorySet && (hb.Category == "coding" || hb.Category == "null")) || (!extraCategorySet && opts.CategorySet))
	if hb.EntityType == "file" && !hb.IsUnsavedEntity {
		if hb.Lines == nil {
			if lines := countLines(statsFile); lines > 0 {
				hb.Lines = &lines
			}
		}
		if hb.ProjectRootCount == nil {
			hb.ProjectRootCount = projectRootCount(root)
		}
		if len(hb.Dependencies) == 0 {
			hb.Dependencies = detectDependenciesForLanguage(statsFile, hb.Language)
		}
	}
	hb = sanitizeHeartbeat(hb, heartbeatSanitizeInput{
		hideBranchNames:   opts.HideBranchNames,
		hideDependencies:  opts.HideDependencies,
		hideFileNames:     opts.HideFileNames,
		hideProjectFolder: opts.HideProjectFolder,
		hideProjectNames:  opts.HideProjectNames,
		projectRoot:       root,
		remoteEntity:      remoteEntity,
	})
	return hb, false, nil
}
