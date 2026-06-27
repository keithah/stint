package services

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func ExtractHeartbeatsFromWakaTimeDump(raw []byte) ([]Heartbeat, error) {
	return ExtractHeartbeatsFromWakaTimeDumpReader(bytes.NewReader(raw), 0)
}

func ExtractHeartbeatsFromWakaTimeDumpReader(src io.Reader, maxBytes int64) ([]Heartbeat, error) {
	reader, closeReader, err := wakaTimeDumpReader(src)
	if err != nil {
		return nil, err
	}
	if closeReader != nil {
		defer closeReader()
	}
	if maxBytes > 0 {
		reader = &limitedDecodeReader{reader: reader, max: maxBytes}
	}
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return nil, errors.New("import file does not contain heartbeat data")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil, errors.New("import file does not contain heartbeat data")
	}
	switch delim {
	case '[':
		var direct []Heartbeat
		for decoder.More() {
			var heartbeat Heartbeat
			if err := decoder.Decode(&heartbeat); err != nil {
				return nil, err
			}
			direct = append(direct, heartbeat)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		if len(direct) == 0 {
			return nil, errors.New("import file does not contain heartbeat data")
		}
		return normalizeImportedHeartbeats(direct), nil
	case '{':
		return extractWakaTimeObjectHeartbeats(decoder)
	default:
		return nil, errors.New("import file does not contain heartbeat data")
	}
}

func wakaTimeDumpReader(src io.Reader) (io.Reader, func() error, error) {
	buffered := bufio.NewReader(src)
	header, _ := buffered.Peek(2)
	if len(header) < 2 || header[0] != 0x1f || header[1] != 0x8b {
		return buffered, nil, nil
	}
	reader, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, nil, err
	}
	return reader, reader.Close, nil
}

func extractWakaTimeObjectHeartbeats(decoder *json.Decoder) ([]Heartbeat, error) {
	var data []Heartbeat
	var heartbeats []Heartbeat
	var days []struct {
		Heartbeats []Heartbeat `json:"heartbeats"`
	}
	var user map[string]any
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("import file does not contain heartbeat data")
		}
		switch key {
		case "data":
			if err := decoder.Decode(&data); err != nil {
				return nil, err
			}
		case "heartbeats":
			if err := decoder.Decode(&heartbeats); err != nil {
				return nil, err
			}
		case "days":
			if err := decoder.Decode(&days); err != nil {
				return nil, err
			}
		case "user":
			if err := decoder.Decode(&user); err != nil {
				return nil, err
			}
		default:
			var discard json.RawMessage
			if err := decoder.Decode(&discard); err != nil {
				return nil, err
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	dayHeartbeats := heartbeatsFromWakaTimeDays(days)
	switch {
	case len(data) > 0:
		return normalizeImportedHeartbeats(data), nil
	case len(heartbeats) > 0:
		return normalizeImportedHeartbeats(heartbeats), nil
	case len(dayHeartbeats) > 0:
		return normalizeImportedHeartbeats(dayHeartbeats), nil
	case len(user) > 0:
		return nil, errors.New("import file contains WakaTime profile metadata but no heartbeat rows; export Heartbeats from WakaTime settings")
	default:
		return nil, errors.New("import file does not contain heartbeat data")
	}
}

type limitedDecodeReader struct {
	reader io.Reader
	max    int64
	read   int64
}

func (r *limitedDecodeReader) Read(p []byte) (int, error) {
	if r.read >= r.max {
		return 0, fmt.Errorf("import file is too large; limit is %d MiB", r.max>>20)
	}
	if remaining := r.max - r.read; int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
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
