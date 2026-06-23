// Command collect scans local AI coding-agent data files, normalizes them to
// canonical usage events, and posts them to the Stint server.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/keithah/stint/internal/collector"
	"github.com/keithah/stint/internal/usage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "collect: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("collect", flag.ContinueOnError)
	apiURL := fs.String("api-url", env("STINT_API_URL", ""), "Stint API base URL")
	apiKey := fs.String("api-key", env("STINT_API_KEY", ""), "Stint API key (Bearer)")
	agent := fs.String("agent", "", "scan only this agent id (default: all registered)")
	once := fs.Bool("once", true, "run a single scan and exit")
	watch := fs.Bool("watch", false, "poll loop: re-scan on an interval")
	interval := fs.Duration("interval", 60*time.Second, "poll interval when --watch is set")
	dryRun := fs.Bool("dry-run", false, "scan and report only; do not POST")
	statePath := fs.String("state", collector.DefaultStatePath(), "incremental state file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reg := collector.DefaultRegistry()

	var ids []string
	if *agent != "" {
		if _, ok := reg[*agent]; !ok {
			return fmt.Errorf("unknown agent %q (registered: %v)", *agent, reg.IDs())
		}
		ids = []string{*agent}
	} else {
		ids = reg.IDs()
	}

	state, err := collector.LoadState(*statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var client *collector.Client
	if !*dryRun {
		if *apiURL == "" || *apiKey == "" {
			return fmt.Errorf("--api-url and --api-key are required (or use --dry-run)")
		}
		client = collector.NewClient(*apiURL, *apiKey)
	}

	scanAndPost := func() error {
		return scanOnce(reg, ids, state, *statePath, client, *dryRun)
	}

	if *watch && !*once {
		for {
			if err := scanAndPost(); err != nil {
				fmt.Fprintln(os.Stderr, "collect: scan error: "+err.Error())
			}
			time.Sleep(*interval)
		}
	}
	return scanAndPost()
}

func scanOnce(reg collector.Registry, ids []string, state *collector.State, statePath string, client *collector.Client, dryRun bool) error {
	var all []usage.Event
	for _, id := range ids {
		entry := reg[id]
		events, report, err := entry.Adapter.Scan(entry.BaseDirs(), state)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collect: agent %s scan error: %v\n", id, err)
		}
		printReport(id, report, len(events))
		all = append(all, events...)
		if dryRun {
			printSample(events)
		}
	}

	all = usage.Dedup(all)
	fmt.Printf("\nTotal deduped events: %d\n", len(all))

	if dryRun {
		fmt.Println("dry-run: not posting")
		return nil
	}

	// Persist state only after a successful post so a failed post is retried.
	ctx := context.Background()
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

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
