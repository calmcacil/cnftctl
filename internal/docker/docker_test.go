package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeDetector struct{ installed, running bool }

func (d fakeDetector) DockerInstalled() bool { return d.installed }
func (d fakeDetector) DockerRunning() bool   { return d.running }

func TestDetect(t *testing.T) {
	got := Detect(fakeDetector{installed: true, running: true})
	if !got.Installed || !got.Running {
		t.Fatalf("unexpected detection: %#v", got)
	}
}

func TestPlanNftablesBackendValueKinds(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKind  BackendValueKind
		wantError string
	}{
		{name: "absent", input: `{}`, wantKind: BackendAbsent},
		{name: "string", input: `{"firewall-backend":"iptables"}`, wantKind: BackendString},
		{name: "empty string", input: `{"firewall-backend":""}`, wantKind: BackendString, wantError: "non-empty string"},
		{name: "null", input: `{"firewall-backend":null}`, wantKind: BackendInvalid, wantError: "must be a string"},
		{name: "boolean", input: `{"firewall-backend":true}`, wantKind: BackendInvalid, wantError: "must be a string"},
		{name: "number", input: `{"firewall-backend":1}`, wantKind: BackendInvalid, wantError: "must be a string"},
		{name: "array", input: `{"firewall-backend":[]}`, wantKind: BackendInvalid, wantError: "must be a string"},
		{name: "object", input: `{"firewall-backend":{}}`, wantKind: BackendInvalid, wantError: "must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := InspectDaemonJSON("daemon.json", []byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			_, kind := FirewallBackend(cfg)
			if kind != tt.wantKind {
				t.Fatalf("kind=%v, want %v", kind, tt.wantKind)
			}
			_, err = PlanNftablesBackend("daemon.json", []byte(tt.input), true, time.Now())
			if tt.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("error=%v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestWritePlanAtomicBackupAndMetadata(t *testing.T) {
	dir := t.TempDir()
	chmodPrivate(t, dir)
	path := filepath.Join(dir, "daemon.json")
	before := []byte(`{"log-level":"info"}`)
	if err := os.WriteFile(path, before, 0o640); err != nil {
		t.Fatal(err)
	}
	oldInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanNftablesBackend(path, before, true, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePlan(plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plan.After) {
		t.Fatalf("destination=%q, want exact plan %q", got, plan.After)
	}
	backup, err := os.ReadFile(plan.BackupPath)
	if err != nil || string(backup) != string(before) {
		t.Fatalf("backup=%q err=%v", backup, err)
	}
	newInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldInfo, newInfo) {
		t.Fatal("destination was modified in place instead of atomically replaced")
	}
	if newInfo.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o, want 640", newInfo.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".daemon.json.*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staging files=%v err=%v", matches, err)
	}
}

func TestWritePlanCreatesNewSecureFileWithoutBackup(t *testing.T) {
	dir := t.TempDir()
	chmodPrivate(t, dir)
	path := filepath.Join(dir, "daemon.json")
	plan, err := PlanNftablesBackend(path, nil, false, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePlan(plan); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(plan.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup for new file: %v", err)
	}
}

func TestWritePlanRejectsConcurrentModification(t *testing.T) {
	dir := t.TempDir()
	chmodPrivate(t, dir)
	path := filepath.Join(dir, "daemon.json")
	before := []byte(`{}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanNftablesBackend(path, before, true, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte(`{"debug":true}`)
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	err = WritePlan(plan)
	if err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("expected concurrent modification error, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(changed) {
		t.Fatalf("concurrent contents overwritten: %q", got)
	}
}

func TestWritePlanRejectsSymlinkAndUnsafeParent(t *testing.T) {
	t.Run("destination symlink", func(t *testing.T) {
		dir := t.TempDir()
		chmodPrivate(t, dir)
		target := filepath.Join(dir, "target")
		path := filepath.Join(dir, "daemon.json")
		if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		plan := EditPlan{Path: path, BackupPath: path + ".bak", Before: []byte(`{}`), BeforeExists: true, After: []byte(`{}`), Changed: true}
		if err := WritePlan(plan); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("expected symlink rejection, got %v", err)
		}
	})
	t.Run("unsafe parent", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "docker")
		if err := os.Mkdir(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "daemon.json")
		plan := EditPlan{Path: path, BackupPath: path + ".bak", After: []byte(`{}`), Changed: true}
		if err := WritePlan(plan); err == nil || !strings.Contains(err.Error(), "world-writable") {
			t.Fatalf("expected unsafe parent rejection, got %v", err)
		}
	})
}

func TestWritePlanUsesCollisionResistantBackup(t *testing.T) {
	dir := t.TempDir()
	chmodPrivate(t, dir)
	path := filepath.Join(dir, "daemon.json")
	before := []byte(`{}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanNftablesBackend(path, before, true, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("do not overwrite")
	if err := os.WriteFile(plan.BackupPath, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WritePlan(plan); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(plan.BackupPath)
	if string(got) != string(marker) {
		t.Fatalf("existing backup overwritten: %q", got)
	}
	got, err = os.ReadFile(plan.BackupPath + ".1")
	if err != nil || string(got) != string(before) {
		t.Fatalf("collision backup=%q err=%v", got, err)
	}
}

func TestWritePlanErrorsLeaveDestinationUntouched(t *testing.T) {
	dir := t.TempDir()
	chmodPrivate(t, dir)
	path := filepath.Join(dir, "daemon.json")
	before := []byte(`{}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	plan := EditPlan{Path: path, BackupPath: filepath.Join(dir, "missing", "backup"), Before: before, BeforeExists: true, After: []byte(`{"firewall-backend":"nftables"}`), Changed: true}
	if err := WritePlan(plan); err == nil {
		t.Fatal("expected backup creation error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(before) {
		t.Fatalf("destination changed after error: %q", got)
	}
}

func TestWritePlanRejectsInvalidPlannedJSON(t *testing.T) {
	dir := t.TempDir()
	chmodPrivate(t, dir)
	path := filepath.Join(dir, "daemon.json")
	plan := EditPlan{Path: path, BackupPath: path + ".bak", After: []byte(`[]`), Changed: true}
	if err := WritePlan(plan); err == nil || !strings.Contains(err.Error(), "exact planned JSON object") {
		t.Fatalf("expected exact JSON object validation error, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid candidate installed: %v", err)
	}
}

func chmodPrivate(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestPlanNftablesBackendPreservesConfigAndRequiresRestart(t *testing.T) {
	now := time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC)
	plan, err := PlanNftablesBackend("/etc/docker/daemon.json", []byte(`{"log-driver":"journald"}`), true, now)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed || !plan.RestartRequired || plan.BackupPath != "/etc/docker/daemon.json.20260709T010203Z.bak" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	after := string(plan.After)
	if !strings.Contains(after, `"firewall-backend": "nftables"`) || !strings.Contains(after, `"log-driver": "journald"`) {
		t.Fatalf("planned JSON lost settings: %s", after)
	}
	if len(plan.Warnings) != 1 || plan.Warnings[0].Code != "docker_restart_required" {
		t.Fatalf("expected no implicit restart warning, got %#v", plan.Warnings)
	}
}

func TestPlanNftablesBackendNoopsWhenAlreadyConfigured(t *testing.T) {
	plan, err := PlanNftablesBackend("/etc/docker/daemon.json", []byte(`{"firewall-backend":"nftables"}`), true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed || plan.RestartRequired || plan.BackupPath != "" {
		t.Fatalf("expected no-op plan, got %#v", plan)
	}
}
