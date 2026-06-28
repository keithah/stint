package stintcli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type fileExpertsResponse struct {
	Data []fileExpertData `json:"data"`
}

type fileExpertData struct {
	Total fileExpertTotal `json:"total"`
	User  fileExpertUser  `json:"user"`
}

type fileExpertTotal struct {
	Decimal      string  `json:"decimal"`
	Digital      string  `json:"digital"`
	Text         string  `json:"text"`
	TotalSeconds float64 `json:"total_seconds"`
}

type fileExpertUser struct {
	ID            string `json:"id"`
	IsCurrentUser bool   `json:"is_current_user"`
	LongName      string `json:"long_name"`
	Name          string `json:"name"`
}

func writeFileExpertsOutput(stdout io.Writer, format string, body []byte) error {
	format = strings.TrimSpace(format)
	var experts fileExpertsResponse
	if err := json.Unmarshal(body, &experts); err != nil {
		return err
	}
	if len(experts.Data) == 0 {
		return nil
	}
	switch format {
	case "raw-json":
		encoded, err := json.Marshal(experts)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(encoded))
		return err
	case "json":
		current, other := fileExpertPair(experts)
		payload := struct {
			CurrentUser *fileExpertData `json:"you,omitempty"`
			OtherUser   *fileExpertData `json:"other,omitempty"`
		}{
			CurrentUser: current,
			OtherUser:   other,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(encoded))
		return err
	case "", "text":
		current, other := fileExpertPair(experts)
		var parts []string
		if current != nil {
			parts = append(parts, "You: "+current.Total.Text)
		}
		if other != nil {
			parts = append(parts, other.User.Name+": "+other.Total.Text)
		}
		if len(parts) == 0 {
			return nil
		}
		_, err := fmt.Fprintln(stdout, strings.Join(parts, " | "))
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func fileExpertPair(experts fileExpertsResponse) (*fileExpertData, *fileExpertData) {
	var current, other *fileExpertData
	for _, expert := range experts.Data {
		if current != nil && other != nil {
			break
		}
		expert := expert
		if expert.User.IsCurrentUser {
			current = &expert
			continue
		}
		if other == nil {
			other = &expert
		}
	}
	return current, other
}
