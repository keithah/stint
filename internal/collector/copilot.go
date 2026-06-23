package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentCopilot = "copilot"

// GitHub Copilot CLI emits OpenTelemetry spans under ~/.copilot/otel/. Each
// file is OTLP/JSON: either a single export request object, or NDJSON with one
// export request per line. The structure follows the OTLP spec:
//
//	{ "resourceSpans": [ {
//	    "resource": { "attributes": [...] },
//	    "scopeSpans": [ {
//	      "scope": {...},
//	      "spans": [ {
//	        "name": "...", "startTimeUnixNano": "...",
//	        "attributes": [ {"key":"gen_ai.usage.input_tokens",
//	                         "value":{"intValue":"123"}}, ... ]
//	      } ]
//	    } ]
//	} ] }
//
// We extract OTEL GenAI semantic-convention attributes from each span and emit
// a usage event per span that carries token counts. Copilot is plan-based, so
// BillingType is "subscription".

// copilotExport is one OTLP/JSON ExportTraceServiceRequest.
type copilotExport struct {
	ResourceSpans []copilotResourceSpans `json:"resourceSpans"`
}

type copilotResourceSpans struct {
	ScopeSpans []copilotScopeSpans `json:"scopeSpans"`
	// Older OTLP/JSON used "instrumentationLibrarySpans"; accept it too.
	InstrumentationLibrarySpans []copilotScopeSpans `json:"instrumentationLibrarySpans"`
}

type copilotScopeSpans struct {
	Spans []copilotSpan `json:"spans"`
}

type copilotSpan struct {
	Name              string             `json:"name"`
	StartTimeUnixNano string             `json:"startTimeUnixNano"`
	SpanID            string             `json:"spanId"`
	TraceID           string             `json:"traceId"`
	Attributes        []copilotAttribute `json:"attributes"`
}

// copilotAttribute is a single OTLP KeyValue. The value is a oneof; we read the
// int/string forms used by the GenAI conventions.
type copilotAttribute struct {
	Key   string `json:"key"`
	Value struct {
		StringValue *string `json:"stringValue"`
		IntValue    *string `json:"intValue"`
		// Some exporters emit a JSON number rather than the spec's stringified
		// int; tolerate both via json.Number on a fallback field.
		IntValueNum *json.Number `json:"intValueNum"`
		DoubleValue *float64     `json:"doubleValue"`
		BoolValue   *bool        `json:"boolValue"`
	} `json:"value"`
}

// scanCopilot implements the Adapter for GitHub Copilot CLI OTEL spans. It walks
// each base dir for span files, reads only the unconsumed tail (per State),
// parses each line/object, maps usage-bearing spans to events, and returns the
// deduped set.
func scanCopilot(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := copilotFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanCopilotFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// copilotFiles returns all OTEL span files under base (recursively): *.json and
// *.ndjson / *.jsonl. A missing base dir yields no files and no error.
func copilotFiles(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if copilotSpanFile(base) {
			return []string{base}, nil
		}
		return nil, nil
	}
	var files []string
	err = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees
		}
		if !d.IsDir() && copilotSpanFile(d.Name()) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

func copilotSpanFile(name string) bool {
	return strings.HasSuffix(name, ".json") ||
		strings.HasSuffix(name, ".ndjson") ||
		strings.HasSuffix(name, ".jsonl")
}

// scanCopilotFile reads the unconsumed portion of one file, appending events and
// updating report + state. It never returns an error; bad lines are counted.
//
// Each line is treated as one OTLP/JSON export request (NDJSON). A
// pretty-printed single-object file is handled by the whole-file fallback when
// the file is not newline-delimited: the first scan reads it as one record.
func scanCopilotFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	pathProject := copilotProjectFromPath(path, base)
	defaultSession := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(
		filepath.Base(path), ".ndjson"), ".jsonl"), ".json")

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		n, perr := parseCopilotExport(line, defaultSession, pathProject, events, report)
		if perr != nil {
			report.Errors++
			report.LinesSkipped++
		} else if n == 0 {
			report.LinesSkipped++
		}
	})
}

// parseCopilotExport parses one OTLP/JSON export request (one NDJSON line, or a
// whole single-object file). It appends one event per usage-bearing span and
// returns the number of events emitted. err!=nil means malformed JSON.
func parseCopilotExport(data []byte, defaultSession, pathProject string, events *[]usage.Event, report *ScanReport) (int, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0, nil
	}
	var exp copilotExport
	if err := json.Unmarshal([]byte(trimmed), &exp); err != nil {
		return 0, err
	}

	emitted := 0
	for _, rs := range exp.ResourceSpans {
		scopes := rs.ScopeSpans
		scopes = append(scopes, rs.InstrumentationLibrarySpans...)
		for _, ss := range scopes {
			for _, sp := range ss.Spans {
				if ev, ok := copilotSpanToEvent(sp, defaultSession, pathProject); ok {
					*events = append(*events, ev)
					report.EventsEmitted++
					emitted++
				}
			}
		}
	}
	return emitted, nil
}

// copilotSpanToEvent maps one span to an event when it carries GenAI token
// usage attributes. ok=false means the span has no usage (not an error).
func copilotSpanToEvent(sp copilotSpan, defaultSession, pathProject string) (usage.Event, bool) {
	attrs := make(map[string]copilotAttribute, len(sp.Attributes))
	for _, a := range sp.Attributes {
		attrs[a.Key] = a
	}

	inputTok := copilotInt(attrs, "gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens")
	outputTok := copilotInt(attrs, "gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens")
	// Cache reads appear under a few spellings depending on the exporter.
	cacheRead := copilotInt(attrs,
		"gen_ai.usage.cache_read_input_tokens",
		"gen_ai.usage.cached_input_tokens",
		"gen_ai.usage.input_tokens.cached")
	cacheCreate := copilotInt(attrs,
		"gen_ai.usage.cache_creation_input_tokens",
		"gen_ai.usage.cache_write_input_tokens")
	reasoning := copilotInt(attrs,
		"gen_ai.usage.reasoning_tokens",
		"gen_ai.usage.output_tokens.reasoning")

	// Model: response model wins over request model.
	model := copilotStr(attrs, "gen_ai.response.model", "gen_ai.request.model")

	ev := usage.Event{
		Agent:       agentCopilot,
		Model:       model,
		BillingType: usage.BillingSubscription,
	}
	tokenUsage{
		Input:         inputTok,
		Output:        outputTok,
		CacheRead:     cacheRead,
		CacheCreate5m: cacheCreate,
		Reasoning:     reasoning,
	}.apply(&ev)

	// Identity: the OTEL span id is a stable per-request key; pair it with the
	// response id when present so dedup collapses re-exported spans.
	ev.MessageID = copilotStr(attrs, "gen_ai.response.id")
	ev.RequestID = sp.SpanID

	// Session/conversation, if the exporter tags one.
	ev.SessionID = copilotStr(attrs, "gen_ai.conversation.id", "session.id")
	if ev.SessionID == "" {
		ev.SessionID = defaultSession
	}
	ev.Project = pathProject

	ev.Timestamp = copilotTimestamp(sp.StartTimeUnixNano)

	if !ev.HasUsage() {
		return usage.Event{}, false
	}
	ev.EnsureID()
	return ev, true
}

// copilotInt returns the first present integer attribute among keys, reading the
// OTLP stringified intValue (and tolerating a numeric form).
func copilotInt(attrs map[string]copilotAttribute, keys ...string) int {
	for _, k := range keys {
		a, ok := attrs[k]
		if !ok {
			continue
		}
		if a.Value.IntValue != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(*a.Value.IntValue)); err == nil {
				return n
			}
		}
		if a.Value.IntValueNum != nil {
			if n, err := a.Value.IntValueNum.Int64(); err == nil {
				return int(n)
			}
		}
		if a.Value.DoubleValue != nil {
			return int(*a.Value.DoubleValue)
		}
	}
	return 0
}

// copilotStr returns the first present string attribute among keys.
func copilotStr(attrs map[string]copilotAttribute, keys ...string) string {
	for _, k := range keys {
		if a, ok := attrs[k]; ok && a.Value.StringValue != nil {
			return *a.Value.StringValue
		}
	}
	return ""
}

// copilotTimestamp converts an OTLP startTimeUnixNano (nanoseconds since epoch,
// stringified per the spec) to RFC3339 UTC. An unparseable/zero value yields "".
func copilotTimestamp(unixNano string) string {
	unixNano = strings.TrimSpace(unixNano)
	if unixNano == "" {
		return ""
	}
	n, err := strconv.ParseInt(unixNano, 10, 64)
	if err != nil {
		return ""
	}
	return normalizeUnixNanos(n)
}

// copilotProjectFromPath derives a fallback project name from the OTEL file
// path. There is no project encoded in ~/.copilot/otel, so it returns the
// immediate parent dir name.
func copilotProjectFromPath(path, base string) string {
	return filepath.Base(filepath.Dir(path))
}
