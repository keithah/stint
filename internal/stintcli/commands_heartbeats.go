package stintcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

func runHeartbeat(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	if opts.Entity == "" {
		return fmt.Errorf("--entity is required")
	}
	return sendHeartbeat(stdin, stdout, opts)
}

func runHeartbeatsList(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--date", args[0]}, args[1:]...)
	}
	date, commonArgs, err := splitDateReadArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/heartbeats"
	if strings.TrimSpace(date) != "" {
		path += "?date=" + url.QueryEscape(strings.TrimSpace(date))
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runFileExperts(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--entity", args[0]}, args[1:]...)
	}
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	return postFileExperts(stdout, opts, opts.Entity, opts.Project)
}

func postFileExperts(stdout io.Writer, opts Options, entity, project string) error {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return fmt.Errorf("--entity is required")
	}
	opts.Entity = entity
	opts.Project = first(project, opts.Project)
	hb, err := BuildHeartbeat(opts)
	if err != nil {
		if errors.Is(err, errHeartbeatFiltered) {
			return nil
		}
		return err
	}
	if !validFileExpertsHeartbeat(hb) {
		return nil
	}
	payload := map[string]any{"entity": hb.Entity}
	if strings.TrimSpace(hb.Project) != "" {
		payload["project"] = hb.Project
	}
	if hb.ProjectRootCount != nil {
		payload["project_root_count"] = hb.ProjectRootCount
	}
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.PostJSON(context.Background(), "/users/current/file_experts", payload)
	if err != nil {
		return err
	}
	return writeFileExpertsOutput(stdout, opts.Output, body)
}

func validFileExpertsHeartbeat(hb Heartbeat) bool {
	return strings.TrimSpace(hb.Entity) != "" &&
		strings.TrimSpace(hb.Project) != "" &&
		hb.ProjectRootCount != nil &&
		*hb.ProjectRootCount > 0
}

func postOfflineHeartbeats(opts Options, heartbeats []Heartbeat) ([]Heartbeat, error) {
	groups, targets, err := heartbeatTargetGroups(opts, heartbeats)
	if err != nil {
		return nil, err
	}
	var requeue []Heartbeat
	for _, target := range targets {
		targetOpts := opts
		targetOpts.APIURL = target.APIURL
		targetOpts.APIKey = target.APIKey
		client, err := NewClient(targetOpts)
		if err != nil {
			return nil, err
		}
		body, err := client.PostJSON(context.Background(), "/users/current/heartbeats.bulk", groups[target])
		if err != nil {
			return nil, err
		}
		groupRequeue, err := offlineRequeueFromBulkResponse(body, groups[target])
		if err != nil {
			return nil, err
		}
		requeue = append(requeue, groupRequeue...)
	}
	return requeue, nil
}
