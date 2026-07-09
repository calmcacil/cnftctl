package docker

import (
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

func TestPlanNftablesBackendPreservesConfigAndRequiresRestart(t *testing.T) {
	now := time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC)
	plan, err := PlanNftablesBackend("/etc/docker/daemon.json", []byte(`{"log-driver":"journald"}`), now)
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
	plan, err := PlanNftablesBackend("/etc/docker/daemon.json", []byte(`{"firewall-backend":"nftables"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed || plan.RestartRequired || plan.BackupPath != "" {
		t.Fatalf("expected no-op plan, got %#v", plan)
	}
}
