package stintcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runDoctor(args []string, stdout io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.Get(context.Background(), "/meta")
	apiErr := err
	lastHeartbeat := "unknown"
	if apiErr == nil {
		date := time.Now().UTC().Format("2006-01-02")
		if heartbeats, err := client.Get(context.Background(), "/users/current/heartbeats?date="+date); err == nil {
			lastHeartbeat = doctorLastHeartbeatStatus(heartbeats)
		}
	}
	count, err := CountQueue(opts.QueuePath)
	doctor := buildDoctorReport(opts, doctorAPIURLSource(args, opts), body, count, err, apiErr, lastHeartbeat)
	if writeErr := writeDoctorOutput(stdout, opts.Output, doctor); writeErr != nil {
		return writeErr
	}
	if len(doctor.Problems) > 0 {
		return errors.New(strings.Join(doctor.Problems, "; "))
	}
	return nil
}

type doctorReport struct {
	Meta                  map[string]any
	OK                    bool
	Problems              []string
	APIReachable          bool
	StintConfigPath       string
	StintConfigPresent    bool
	WakaTimeConfigPath    string
	WakaTimeConfigPresent bool
	APIURLSource          string
	WakaTimeCLIPath       string
	WakaTimeCLIStatus     string
	OfflineQueueCount     int
	OfflineQueueError     string
	LastHeartbeat         string
	DetectedEditors       []string
	DetectedAgents        []string
}

func buildDoctorReport(opts Options, apiURLSource string, body []byte, offlineCount int, countErr, apiErr error, lastHeartbeat string) doctorReport {
	report := doctorReport{
		Meta:               map[string]any{},
		APIReachable:       apiErr == nil,
		StintConfigPath:    DefaultStintConfigPath(),
		WakaTimeConfigPath: DefaultWakaTimeConfigPath(),
		APIURLSource:       apiURLSource,
		OfflineQueueCount:  offlineCount,
		LastHeartbeat:      lastHeartbeat,
		DetectedEditors:    DefaultEditorRegistry().DetectInstalled(),
		DetectedAgents:     detectedPluginAgents(),
	}
	_ = json.Unmarshal(body, &report.Meta)
	report.StintConfigPresent = fileExists(report.StintConfigPath)
	report.WakaTimeConfigPresent = fileExists(report.WakaTimeConfigPath)
	if countErr != nil {
		report.OfflineQueueError = countErr.Error()
		report.Problems = append(report.Problems, "offline queue unreadable: "+countErr.Error())
	}
	report.WakaTimeCLIPath, report.WakaTimeCLIStatus = doctorWakaTimeCLIStatus()
	if !report.StintConfigPresent && !report.WakaTimeConfigPresent && opts.APIKey == "" {
		report.Problems = append(report.Problems, "no Stint or WakaTime config found")
	}
	if apiErr != nil {
		report.Problems = append(report.Problems, "api unreachable: "+apiErr.Error())
	}
	if report.WakaTimeCLIStatus == "missing" {
		report.Problems = append(report.Problems, "wakatime-cli is not installed")
	}
	report.OK = len(report.Problems) == 0
	return report
}

func writeDoctorOutput(stdout io.Writer, format string, report doctorReport) error {
	format = strings.TrimSpace(format)
	if format == "json" || format == "raw-json" {
		payload := map[string]any{}
		for key, value := range report.Meta {
			payload[key] = value
		}
		payload["ok"] = report.OK
		payload["problems"] = report.Problems
		payload["api_reachable"] = report.APIReachable
		payload["stint_config_path"] = report.StintConfigPath
		payload["stint_config_present"] = report.StintConfigPresent
		payload["wakatime_config_path"] = report.WakaTimeConfigPath
		payload["wakatime_config_present"] = report.WakaTimeConfigPresent
		payload["api_url_source"] = report.APIURLSource
		payload["wakatime_cli_path"] = report.WakaTimeCLIPath
		payload["wakatime_cli_status"] = report.WakaTimeCLIStatus
		payload["offline_queue_count"] = report.OfflineQueueCount
		payload["offline_queue_error"] = report.OfflineQueueError
		payload["last_heartbeat"] = report.LastHeartbeat
		payload["detected_editors"] = report.DetectedEditors
		payload["detected_agents"] = report.DetectedAgents
		encoded, err := marshalJSONNoHTMLEscape(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(encoded))
		return err
	}
	fmt.Fprintf(stdout, "ok=%t\n", report.OK)
	fmt.Fprintf(stdout, "api_reachable=%t\n", report.APIReachable)
	fmt.Fprintf(stdout, "api_url_source=%s\n", report.APIURLSource)
	fmt.Fprintf(stdout, "stint_config=%s present=%t\n", report.StintConfigPath, report.StintConfigPresent)
	fmt.Fprintf(stdout, "wakatime_config=%s present=%t\n", report.WakaTimeConfigPath, report.WakaTimeConfigPresent)
	fmt.Fprintf(stdout, "wakatime_cli=%s status=%s\n", report.WakaTimeCLIPath, report.WakaTimeCLIStatus)
	fmt.Fprintf(stdout, "offline_queue_count=%d\n", report.OfflineQueueCount)
	if report.OfflineQueueError != "" {
		fmt.Fprintf(stdout, "offline_queue_error=%s\n", report.OfflineQueueError)
	}
	fmt.Fprintf(stdout, "last_heartbeat=%s\n", report.LastHeartbeat)
	fmt.Fprintf(stdout, "detected_editors=%s\n", strings.Join(report.DetectedEditors, ","))
	fmt.Fprintf(stdout, "detected_agents=%s\n", strings.Join(report.DetectedAgents, ","))
	if len(report.Problems) > 0 {
		fmt.Fprintf(stdout, "problems=%s\n", strings.Join(report.Problems, "; "))
	}
	return nil
}

func doctorAPIURLSource(args []string, opts Options) string {
	switch {
	case hasValueFlag(args, "api-url", "apiurl"):
		return "flag"
	case os.Getenv("STINT_API_URL") != "":
		return "env"
	case configHasAPIURL(DefaultStintConfigPath()):
		return "stint_config"
	case configFirst(opts.Config, "api_url", "api-url", "apiurl") != "":
		return "wakatime_config"
	default:
		return "default"
	}
}

func hasValueFlag(args []string, names ...string) bool {
	want := map[string]bool{}
	for _, name := range names {
		want["--"+name] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if before, after, ok := strings.Cut(arg, "="); ok {
			if want[before] && strings.TrimSpace(after) != "" {
				return true
			}
			continue
		}
		if want[arg] && i+1 < len(args) && strings.TrimSpace(args[i+1]) != "" {
			return true
		}
	}
	return false
}

func configHasAPIURL(path string) bool {
	cfg, _ := LoadConfig(path)
	return configFirst(cfg, "api_url", "api-url", "apiurl") != ""
}

func doctorWakaTimeCLIStatus() (string, string) {
	if override := strings.TrimSpace(os.Getenv("STINT_WAKATIME_CLI")); override != "" {
		return expandHome(override), "override"
	}
	path := filepath.Join(wakaResourcesDir(), executableName("wakatime-cli"))
	if fileExists(path) {
		return path, "installed"
	}
	return path, "missing"
}

func doctorLastHeartbeatStatus(body []byte) string {
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil || len(rows) == 0 {
		return "none"
	}
	if entity, _ := rows[0]["entity"].(string); entity != "" {
		return entity
	}
	return "present"
}

func detectedPluginAgents() []string {
	reg := DefaultPluginRegistry()
	var ids []string
	for _, id := range reg.IDs() {
		spec := reg[id]
		if _, err := pluginLookPath(spec.Binary); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
