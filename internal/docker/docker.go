package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
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
	BeforeExists    bool
	After           []byte
	Changed         bool
	RestartRequired bool
	Warnings        []Warning
}

type BackendValueKind int

const (
	BackendAbsent BackendValueKind = iota
	BackendString
	BackendInvalid
)

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

func FirewallBackend(cfg DaemonConfig) (string, BackendValueKind) {
	value, ok := cfg.Data["firewall-backend"]
	if !ok {
		return "", BackendAbsent
	}
	backend, valid := value.(string)
	if !valid {
		return "", BackendInvalid
	}
	return backend, BackendString
}

func PlanNftablesBackend(path string, before []byte, beforeExists bool, now time.Time) (EditPlan, error) {
	cfg, err := InspectDaemonJSON(path, before)
	if err != nil {
		return EditPlan{}, err
	}
	backend, kind := FirewallBackend(cfg)
	if kind == BackendInvalid {
		return EditPlan{}, errors.New("daemon.json firewall-backend must be a string")
	}
	if kind == BackendString && backend == FirewallBackendNftables {
		return EditPlan{Path: cfg.Path, Before: append([]byte(nil), before...), BeforeExists: beforeExists}, nil
	}
	if kind == BackendString && backend == "" {
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
		BeforeExists:    beforeExists,
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
	parent := filepath.Dir(plan.Path)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect daemon.json parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return errors.New("daemon.json parent must be a real directory, not a symlink")
	}
	if parentInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("daemon.json parent %s is group- or world-writable", parent)
	}

	mode := os.FileMode(0o600)
	uid, gid := -1, -1
	current, err := os.Lstat(plan.Path)
	switch {
	case plan.BeforeExists && err != nil:
		return fmt.Errorf("daemon.json changed since it was inspected: %w", err)
	case !plan.BeforeExists && err == nil:
		return errors.New("daemon.json was created since it was inspected")
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect daemon.json destination: %w", err)
	case err == nil:
		if !current.Mode().IsRegular() {
			return errors.New("daemon.json destination must be a regular file and not a symlink")
		}
		data, readErr := os.ReadFile(plan.Path)
		if readErr != nil {
			return fmt.Errorf("re-read daemon.json: %w", readErr)
		}
		if !bytesEqual(data, plan.Before) {
			return errors.New("daemon.json changed since it was inspected")
		}
		mode = current.Mode().Perm() &^ 0o022
		if stat, ok := current.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(stat.Uid), int(stat.Gid)
		}
	}

	if plan.BeforeExists {
		backup, backupPath, createErr := createBackup(plan.BackupPath, mode)
		if createErr != nil {
			return createErr
		}
		backupOK := false
		defer func() {
			if !backupOK {
				_ = os.Remove(backupPath)
			}
		}()
		if err := setOwner(backup, uid, gid); err != nil {
			_ = backup.Close()
			return err
		}
		if _, err := backup.Write(plan.Before); err != nil {
			_ = backup.Close()
			return fmt.Errorf("write daemon.json backup: %w", err)
		}
		if err := backup.Sync(); err != nil {
			_ = backup.Close()
			return fmt.Errorf("sync daemon.json backup: %w", err)
		}
		if err := backup.Close(); err != nil {
			return fmt.Errorf("close daemon.json backup: %w", err)
		}
		backupOK = true
	}

	staged, err := os.CreateTemp(parent, ".daemon.json.*.tmp")
	if err != nil {
		return fmt.Errorf("stage daemon.json: %w", err)
	}
	stagePath := staged.Name()
	keepStage := false
	defer func() {
		if !keepStage {
			_ = os.Remove(stagePath)
		}
	}()
	if err := staged.Chmod(mode); err != nil {
		_ = staged.Close()
		return fmt.Errorf("set staged daemon.json mode: %w", err)
	}
	if err := setOwner(staged, uid, gid); err != nil {
		_ = staged.Close()
		return err
	}
	if _, err := staged.Write(plan.After); err != nil {
		_ = staged.Close()
		return fmt.Errorf("write staged daemon.json: %w", err)
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return fmt.Errorf("sync staged daemon.json: %w", err)
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		_ = staged.Close()
		return fmt.Errorf("rewind staged daemon.json: %w", err)
	}
	stagedData, err := io.ReadAll(staged)
	if err != nil {
		_ = staged.Close()
		return fmt.Errorf("validate staged daemon.json: %w", err)
	}
	var exact map[string]any
	if !bytesEqual(stagedData, plan.After) || json.Unmarshal(stagedData, &exact) != nil || exact == nil {
		_ = staged.Close()
		return errors.New("staged daemon.json did not validate as the exact planned JSON object")
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged daemon.json: %w", err)
	}
	if err := os.Rename(stagePath, plan.Path); err != nil {
		return fmt.Errorf("replace daemon.json atomically: %w", err)
	}
	keepStage = true
	dir, err := os.Open(parent)
	if err != nil {
		return fmt.Errorf("open daemon.json parent for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync daemon.json parent: %w", err)
	}
	return nil
}

func createBackup(base string, mode os.FileMode) (*os.File, string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		path := base
		if attempt > 0 {
			path = fmt.Sprintf("%s.%d", base, attempt)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			return file, path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", fmt.Errorf("create daemon.json backup: %w", err)
		}
	}
	return nil, "", errors.New("could not create a unique daemon.json backup")
}

func setOwner(file *os.File, uid, gid int) error {
	if uid < 0 || gid < 0 {
		return nil
	}
	if err := file.Chown(uid, gid); err != nil {
		return fmt.Errorf("preserve daemon.json ownership: %w", err)
	}
	return nil
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
