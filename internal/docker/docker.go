package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const FirewallBackendNftables = "nftables"

type Detector interface {
	DockerInstalled() bool
	DockerRunning() bool
}

type Detection struct {
	Installed bool
	Running   bool
}

type DaemonConfig struct {
	Path string
	Data map[string]any
}

type EditPlan struct {
	Path            string
	BackupPath      string
	Before          []byte
	After           []byte
	Changed         bool
	RestartRequired bool
	Warnings        []Warning
}

type Warning struct {
	Code    string
	Message string
}

func Detect(detector Detector) Detection {
	if detector == nil {
		return Detection{}
	}
	return Detection{Installed: detector.DockerInstalled(), Running: detector.DockerRunning()}
}

func InspectDaemonJSON(path string, data []byte) (DaemonConfig, error) {
	if path == "" {
		path = "/etc/docker/daemon.json"
	}
	if len(data) == 0 {
		return DaemonConfig{Path: path, Data: map[string]any{}}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return DaemonConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if parsed == nil {
		parsed = map[string]any{}
	}
	return DaemonConfig{Path: path, Data: parsed}, nil
}

func FirewallBackend(cfg DaemonConfig) (string, bool) {
	value, ok := cfg.Data["firewall-backend"]
	if !ok {
		return "", false
	}
	backend, ok := value.(string)
	return backend, ok
}

func PlanNftablesBackend(path string, before []byte, now time.Time) (EditPlan, error) {
	cfg, err := InspectDaemonJSON(path, before)
	if err != nil {
		return EditPlan{}, err
	}
	backend, ok := FirewallBackend(cfg)
	if ok && backend == FirewallBackendNftables {
		return EditPlan{Path: cfg.Path, Before: append([]byte(nil), before...)}, nil
	}
	if ok && backend == "" {
		return EditPlan{}, errors.New("daemon.json firewall-backend must be a non-empty string")
	}

	afterData := cloneMap(cfg.Data)
	afterData["firewall-backend"] = FirewallBackendNftables
	after, err := marshalStableJSON(afterData)
	if err != nil {
		return EditPlan{}, err
	}

	backupPath := cfg.Path + "." + now.UTC().Format("20060102T150405Z") + ".bak"
	return EditPlan{
		Path:            cfg.Path,
		BackupPath:      backupPath,
		Before:          append([]byte(nil), before...),
		After:           after,
		Changed:         true,
		RestartRequired: true,
		Warnings: []Warning{{
			Code:    "docker_restart_required",
			Message: "Changing Docker's firewall backend requires an explicit Docker restart; cnftctl must not restart Docker implicitly.",
		}},
	}, nil
}

func WritePlan(plan EditPlan) error {
	if !plan.Changed {
		return nil
	}
	if plan.Path == "" || plan.BackupPath == "" {
		return errors.New("edit plan is missing path or backup path")
	}
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return err
	}
	if len(plan.Before) > 0 {
		if err := os.WriteFile(plan.BackupPath, plan.Before, 0o600); err != nil {
			return err
		}
	}
	return os.WriteFile(plan.Path, plan.After, 0o600)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func marshalStableJSON(data map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	ordered := make(map[string]any, len(data))
	for _, key := range keys {
		ordered[key] = data[key]
	}
	return json.MarshalIndent(ordered, "", "  ")
}
