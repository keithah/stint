package services

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
)

func ExtractHeartbeatsFromWakaTimeDump(raw []byte) ([]Heartbeat, error) {
	raw, err := maybeGunzipWakaTimeDump(raw)
	if err != nil {
		return nil, err
	}
	var direct []Heartbeat
	if err := json.Unmarshal(raw, &direct); err == nil && len(direct) > 0 {
		return normalizeImportedHeartbeats(direct), nil
	}

	var wrapped struct {
		Data       []Heartbeat `json:"data"`
		Heartbeats []Heartbeat `json:"heartbeats"`
		Days       []struct {
			Heartbeats []Heartbeat `json:"heartbeats"`
		} `json:"days"`
		User map[string]any `json:"user"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	dayHeartbeats := heartbeatsFromWakaTimeDays(wrapped.Days)
	switch {
	case len(wrapped.Data) > 0:
		return normalizeImportedHeartbeats(wrapped.Data), nil
	case len(wrapped.Heartbeats) > 0:
		return normalizeImportedHeartbeats(wrapped.Heartbeats), nil
	case len(dayHeartbeats) > 0:
		return normalizeImportedHeartbeats(dayHeartbeats), nil
	case len(wrapped.User) > 0:
		return nil, errors.New("import file contains WakaTime profile metadata but no heartbeat rows; export Heartbeats from WakaTime settings")
	default:
		return nil, errors.New("import file does not contain heartbeat data")
	}
}

func maybeGunzipWakaTimeDump(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return raw, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func normalizeImportedHeartbeats(heartbeats []Heartbeat) []Heartbeat {
	out := make([]Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		if heartbeat.Type == "" {
			heartbeat.Type = "file"
		}
		out = append(out, heartbeat)
	}
	return out
}

func heartbeatsFromWakaTimeDays(days []struct {
	Heartbeats []Heartbeat `json:"heartbeats"`
}) []Heartbeat {
	total := 0
	for _, day := range days {
		total += len(day.Heartbeats)
	}
	if total == 0 {
		return nil
	}
	out := make([]Heartbeat, 0, total)
	for _, day := range days {
		out = append(out, day.Heartbeats...)
	}
	return out
}
