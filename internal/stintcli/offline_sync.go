package stintcli

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type bulkHeartbeatResponse struct {
	Responses []json.RawMessage `json:"responses"`
}

func offlineRequeueFromBulkResponse(body []byte, heartbeats []Heartbeat) ([]Heartbeat, error) {
	var response bulkHeartbeatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse bulk heartbeat response: %w", err)
	}
	if response.Responses == nil {
		return nil, nil
	}

	handled := map[string]int{}
	requeue := make([]Heartbeat, 0)
	for i, raw := range response.Responses {
		hb, ok := responseHeartbeat(raw, heartbeats, i)
		if !ok {
			requeue = append(requeue, missingResponseHeartbeats(heartbeats, handled)...)
			return requeue, nil
		}
		handled[hb.ID()]++

		status, ok := responseStatus(raw)
		if !ok {
			requeue = append(requeue, hb)
			continue
		}
		if status == http.StatusBadRequest {
			continue
		}
		if status < http.StatusOK || status > 299 {
			requeue = append(requeue, hb)
		}
	}
	requeue = append(requeue, missingResponseHeartbeats(heartbeats, handled)...)
	return requeue, nil
}

func responseHeartbeat(raw json.RawMessage, heartbeats []Heartbeat, index int) (Heartbeat, bool) {
	var row []json.RawMessage
	if err := json.Unmarshal(raw, &row); err == nil && len(row) > 0 {
		if hb, ok := heartbeatFromResponseObject(row[0]); ok {
			return hb, true
		}
	}
	if hb, ok := heartbeatFromResponseObject(raw); ok {
		return hb, true
	}
	if index >= len(heartbeats) {
		return Heartbeat{}, false
	}
	return heartbeats[index], true
}

func heartbeatFromResponseObject(raw json.RawMessage) (Heartbeat, bool) {
	var wrapper struct {
		Data      Heartbeat `json:"data"`
		Heartbeat Heartbeat `json:"heartbeat"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return Heartbeat{}, false
	}
	if wrapper.Heartbeat.Entity != "" || wrapper.Heartbeat.Time != 0 {
		return wrapper.Heartbeat, true
	}
	if wrapper.Data.Entity != "" || wrapper.Data.Time != 0 {
		return wrapper.Data, true
	}
	return Heartbeat{}, false
}

func responseStatus(raw json.RawMessage) (int, bool) {
	var row []json.RawMessage
	if err := json.Unmarshal(raw, &row); err == nil {
		if len(row) > 1 {
			var status int
			if err := json.Unmarshal(row[1], &status); err == nil {
				return status, true
			}
		}
		if len(row) > 0 {
			return responseStatus(row[0])
		}
	}
	var obj struct {
		Status     int `json:"status"`
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return 0, false
	}
	if obj.Status != 0 {
		return obj.Status, true
	}
	if obj.StatusCode != 0 {
		return obj.StatusCode, true
	}
	return 0, false
}

func missingResponseHeartbeats(heartbeats []Heartbeat, handled map[string]int) []Heartbeat {
	missing := make([]Heartbeat, 0)
	for _, hb := range heartbeats {
		id := hb.ID()
		if handled[id] > 0 {
			handled[id]--
			continue
		}
		missing = append(missing, hb)
	}
	return missing
}
