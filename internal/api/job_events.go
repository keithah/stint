package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/keithah/stint/internal/db"
	"github.com/labstack/echo/v4"
)

const currentUserEventsInterval = 5 * time.Second

func (s *Server) currentUserEvents(c echo.Context) error {
	user := userFromContext(c)
	response := c.Response()
	flusher, ok := response.Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, errorBody("streaming is not supported"))
	}
	response.Header().Set(echo.HeaderContentType, "text/event-stream")
	response.Header().Set(echo.HeaderCacheControl, "no-cache")
	response.Header().Set("Connection", "keep-alive")
	response.Header().Set("X-Accel-Buffering", "no")
	response.WriteHeader(http.StatusOK)

	writeEvent := func(name string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(response, "event: %s\ndata: %s\n\n", name, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	var lastDumps, lastProgress string
	sendChanged := func(force bool) error {
		dumps, err := s.Store.ListDataDumps(c.Request().Context(), user.ID)
		if err != nil {
			return err
		}
		if version := dataDumpEventsVersion(dumps); force || version != lastDumps {
			lastDumps = version
			if err := writeEvent("data_dumps", map[string]any{"version": version}); err != nil {
				return err
			}
		}
		progress, err := s.Store.GetCustomRulesProgress(c.Request().Context(), user.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			progress = db.CustomRulesProgress{Status: "NotStarted"}
		} else if err != nil {
			return err
		}
		if version := customRulesProgressEventsVersion(progress); force || version != lastProgress {
			lastProgress = version
			if err := writeEvent("custom_rules_progress", map[string]any{"version": version}); err != nil {
				return err
			}
		}
		return nil
	}

	if err := writeEvent("ready", map[string]any{"ok": true}); err != nil {
		return nil
	}
	if err := sendChanged(true); err != nil {
		_ = writeEvent("error", map[string]any{"error": err.Error()})
		return nil
	}

	ticker := time.NewTicker(currentUserEventsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-ticker.C:
			if err := sendChanged(false); err != nil {
				_ = writeEvent("error", map[string]any{"error": err.Error()})
				return nil
			}
		}
	}
}

func dataDumpEventsVersion(dumps []db.DataDump) string {
	type dumpVersion struct {
		ID              uuid.UUID `json:"id"`
		Status          string    `json:"status"`
		PercentComplete float64   `json:"percent_complete"`
		IsProcessing    bool      `json:"is_processing"`
		IsStuck         bool      `json:"is_stuck"`
		HasFailed       bool      `json:"has_failed"`
		DownloadURL     string    `json:"download_url,omitempty"`
	}
	values := make([]dumpVersion, 0, len(dumps))
	for _, dump := range dumps {
		values = append(values, dumpVersion{
			ID:              dump.ID,
			Status:          dump.Status,
			PercentComplete: dump.PercentComplete,
			IsProcessing:    dump.IsProcessing,
			IsStuck:         dump.IsStuck,
			HasFailed:       dump.HasFailed,
			DownloadURL:     dump.DownloadURL,
		})
	}
	data, _ := json.Marshal(values)
	return string(data)
}

func customRulesProgressEventsVersion(progress db.CustomRulesProgress) string {
	data, _ := json.Marshal(struct {
		Status          string `json:"status"`
		PercentComplete int    `json:"percent_complete"`
		Total           int    `json:"total"`
		Changed         int    `json:"changed"`
		Deleted         int    `json:"deleted"`
		Error           string `json:"error,omitempty"`
	}{
		Status:          progress.Status,
		PercentComplete: progress.PercentComplete,
		Total:           progress.Total,
		Changed:         progress.Changed,
		Deleted:         progress.Deleted,
		Error:           progress.Error,
	})
	return string(data)
}
