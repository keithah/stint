// Command collect scans local AI coding-agent data files, normalizes them to
// canonical usage events, and posts them to the Stint server.
//
// Configuration is resolved from, in increasing precedence: built-in defaults,
// a JSON config file (~/.stint/collect.json by default, override with
// --config), environment variables (STINT_API_URL, STINT_API_KEY, ...), then
// explicit command-line flags. See cmd/collect/README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/keithah/stint/internal/collector"
	"github.com/keithah/stint/internal/usage"
)

const collectUploadTimeout = 5 * time.Minute

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "collect: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	// `config init` subcommand: write a starter config and exit.
	if len(args) >= 2 && args[0] == "config" && args[1] == "init" {
		return doConfigInit(args[2:])
	}

	fl, flagSet, err := parseFlags(args)
	if err != nil {
		return err
	}

	// --init-config: write starter config to the resolved config path, then exit.
	if fl.initConfig {
		return initConfigAt(fl.configPath)
	}

	file, fileFound, err := loadConfigFile(fl.configPath, flagSet["config"])
	if err != nil {
		return err
	}

	cfg, err := resolveConfig(file, fileFound, fl, flagSet)
	if err != nil {
		return err
	}
	cfg.ConfigPath = collector.ExpandHome(fl.configPath)

	if fl.printConfig {
		src := "built-in defaults"
		if fileFound {
			src = cfg.ConfigPath
		}
		fmt.Printf("# resolved config (file: %s)\n%s\n", src, configJSON(cfg))
		return nil
	}

	reg := collector.DefaultRegistry()

	ids, err := selectAgents(reg, cfg.Agents)
	if err != nil {
		return err
	}

	state, err := collector.LoadState(cfg.StatePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var client *collector.Client
	if !cfg.DryRun {
		if cfg.APIURL == "" || cfg.APIKey == "" {
			return fmt.Errorf("api_url and api_key are required (set via --api-url/--api-key, env, or config; or use --dry-run)")
		}
		client = collector.NewClient(cfg.APIURL, cfg.APIKey)
	}

	scanAndPost := func() error {
		return scanOnce(reg, ids, cfg.AgentPaths, state, client, cfg.DryRun)
	}

	if cfg.Watch && !cfg.Once {
		fmt.Printf("watch: scanning %d agent(s) every %s (state: %s)\n", len(ids), cfg.Interval, cfg.StatePath)
		for {
			start := time.Now()
			if err := scanAndPost(); err != nil {
				fmt.Fprintln(os.Stderr, "collect: scan error: "+err.Error())
			}
			fmt.Printf("--- cycle done in %s; sleeping %s ---\n", time.Since(start).Round(time.Millisecond), cfg.Interval)
			time.Sleep(cfg.Interval)
		}
	}
	return scanAndPost()
}

// parseFlags defines and parses the CLI flags. The returned flagSet records
// which flags the user explicitly provided so resolveConfig can apply correct
// precedence (a flag left at its zero/default value must not override a config
// file or env value).
func parseFlags(args []string) (*flags, map[string]bool, error) {
	fs := flag.NewFlagSet("collect", flag.ContinueOnError)
	fl := &flags{}
	fs.StringVar(&fl.apiURL, "api-url", "", "Stint API base URL")
	fs.StringVar(&fl.apiKey, "api-key", "", "Stint API key (Bearer)")
	fs.StringVar(&fl.costMode, "cost-mode", "", "cost mode hint (calculate|provided)")
	fs.StringVar(&fl.statePath, "state", "", "incremental state file path")
	fs.StringVar(&fl.interval, "interval", "", "poll interval when --watch is set (e.g. 5m)")
	fs.StringVar(&fl.agent, "agent", "", "scan only this agent id (default: config/all registered)")
	fs.BoolVar(&fl.once, "once", true, "run a single scan and exit")
	fs.BoolVar(&fl.watch, "watch", false, "poll loop: re-scan on an interval")
	fs.BoolVar(&fl.dryRun, "dry-run", false, "scan and report only; do not POST")
	fs.StringVar(&fl.configPath, "config", DefaultConfigPath(), "config file path")
	fs.BoolVar(&fl.printConfig, "print-config", false, "print the resolved config and exit")
	fs.BoolVar(&fl.initConfig, "init-config", false, "write a starter config to --config if absent, then exit")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	flagSet := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { flagSet[f.Name] = true })
	return fl, flagSet, nil
}

// doConfigInit handles `collect config init [--config PATH]`.
func doConfigInit(args []string) error {
	fs := flag.NewFlagSet("collect config init", flag.ContinueOnError)
	path := fs.String("config", DefaultConfigPath(), "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return initConfigAt(*path)
}

func initConfigAt(path string) error {
	wrote, err := writeStarterConfig(path)
	if err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	expanded := collector.ExpandHome(path)
	if wrote {
		fmt.Printf("wrote starter config: %s\n", expanded)
	} else {
		fmt.Printf("config already exists, left unchanged: %s\n", expanded)
	}
	return nil
}

// selectAgents resolves the agent allowlist against the registry, returning ids
// in stable sorted order. An empty allowlist means all registered agents.
func selectAgents(reg collector.Registry, allow []string) ([]string, error) {
	if len(allow) == 0 {
		return reg.IDs(), nil
	}
	ids := make([]string, 0, len(allow))
	for _, id := range allow {
		if _, ok := reg[id]; !ok {
			return nil, fmt.Errorf("unknown agent %q (registered: %v)", id, reg.IDs())
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func scanOnce(reg collector.Registry, ids []string, agentPaths map[string][]string, state *collector.State, client *collector.Client, dryRun bool) error {
	type scanResult struct {
		id     string
		events []usage.Event
		report collector.ScanReport
		err    error
	}
	results := make([]scanResult, len(ids))
	const maxConcurrentScans = 4
	sem := make(chan struct{}, maxConcurrentScans)
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			entry := reg[id]
			baseDirs := entry.BaseDirs()
			if override := agentPaths[id]; len(override) > 0 {
				baseDirs = override
			}
			events, report, err := entry.Adapter.Scan(baseDirs, state)
			results[i] = scanResult{id: id, events: events, report: report, err: err}
		}()
	}
	wg.Wait()

	var all []usage.Event
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "collect: agent %s scan error: %v\n", result.id, result.err)
		}
		printReport(result.id, result.report, len(result.events))
		all = append(all, result.events...)
		if dryRun {
			printSample(result.events)
		}
	}

	all = usage.Dedup(all)
	fmt.Printf("\nTotal deduped events: %d\n", len(all))

	if dryRun {
		fmt.Println("dry-run: not posting")
		return nil
	}

	// Persist state only after a successful post so a failed post is retried.
	ctx, cancelUpload := context.WithTimeout(context.Background(), collectUploadTimeout)
	defer cancelUpload()
	res, err := client.Post(ctx, all)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	if err := state.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "collect: warning: save state: %v\n", err)
	}
	fmt.Printf("Ingest: received=%d inserted=%d duplicates=%d invalid=%d\n",
		res.Received, res.Inserted, res.Duplicates, res.Invalid)
	return nil
}

func printReport(id string, r collector.ScanReport, deduped int) {
	note := ""
	if r.Note != "" {
		note = " (" + r.Note + ")"
	}
	fmt.Printf("[%s]%s files=%d lines=%d emitted=%d deduped=%d skipped=%d errors=%d\n",
		id, note, r.FilesScanned, r.LinesParsed, r.EventsEmitted, deduped, r.LinesSkipped, r.Errors)
}

func printSample(events []usage.Event) {
	if len(events) == 0 {
		return
	}
	sorted := append([]usage.Event(nil), events...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	e := sorted[0]
	fmt.Printf("  sample: model=%s session=%s project=%s in=%d out=%d c5m=%d c1h=%d cr=%d ts=%s\n",
		e.Model, e.SessionID, e.Project, e.InputTokens, e.OutputTokens,
		e.CacheCreate5mTokens, e.CacheCreate1hTokens, e.CacheReadTokens, e.Timestamp)
}
