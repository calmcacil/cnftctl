package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calmcacil/cnftctl/internal/app"
	"github.com/calmcacil/cnftctl/internal/config"
)

type recordingService struct {
	request app.CommandRequest
	err     error
}

func (s *recordingService) Run(_ context.Context, _ app.IO, request app.CommandRequest) error {
	s.request = request
	return s.err
}

func TestInspectionOutputAndHealthExit(t *testing.T) {
	service := &recordingService{err: app.HealthError{State: app.StatePending}}
	var stdout, stderr strings.Builder
	runner := New(Options{Stdout: &stdout, Stderr: &stderr, Service: service})
	if code := runner.Run([]string{"--output", "json", "--detail", "status"}); code != 1 {
		t.Fatalf("Run() code = %d, want 1", code)
	}
	if got := service.request.Flag("output"); got != "json" {
		t.Fatalf("output = %q, want json", got)
	}
	if !service.request.BoolFlag("detail") || stderr.String() != "" {
		t.Fatalf("detail=%t stderr=%q", service.request.BoolFlag("detail"), stderr.String())
	}
}

func TestRootHelp(t *testing.T) {
	var stdout, stderr strings.Builder
	runner := New(Options{Stdout: &stdout, Stderr: &stderr, Version: "test"})

	if code := runner.Run(nil); code != 0 {
		t.Fatalf("Run() code = %d, want 0", code)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{"Usage:", "cnftctl <command>", "status", "preset"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestVersion(t *testing.T) {
	var stdout strings.Builder
	runner := New(Options{Stdout: &stdout, Version: "1.2.3"})

	if code := runner.Run([]string{"--version"}); code != 0 {
		t.Fatalf("Run() code = %d, want 0", code)
	}
	if got, want := strings.TrimSpace(stdout.String()), "cnftctl 1.2.3"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestCommandDispatchWithFlags(t *testing.T) {
	var stdout strings.Builder
	service := &recordingService{}
	runner := New(Options{Stdout: &stdout, Service: service})

	args := []string{"--root", "/tmp/root", "init", "--dry-run", "--wan-interface", "eth0", "--trust-interface", "tailscale0", "--trust-interface=wg0"}
	if code := runner.Run(args); code != 0 {
		t.Fatalf("Run() code = %d, want 0", code)
	}
	if got, want := service.request.Command, "init"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := service.request.Flag("root"), "/tmp/root"; got != want {
		t.Fatalf("root flag = %q, want %q", got, want)
	}
	if !service.request.BoolFlag("dry-run") {
		t.Fatalf("dry-run flag = false, want true")
	}
	if got, want := service.request.Flag("wan-interface"), "eth0"; got != want {
		t.Fatalf("wan-interface flag = %q, want %q", got, want)
	}
	if got, want := strings.Join(service.request.FlagValues("trust-interface"), ","), "tailscale0,wg0"; got != want {
		t.Fatalf("trust-interface values = %q, want %q", got, want)
	}
}

func TestNestedCommandDispatch(t *testing.T) {
	service := &recordingService{}
	runner := New(Options{Service: service})

	if code := runner.Run([]string{"feature", "enable", "trusted-interface", "--interface", "tailscale0"}); code != 0 {
		t.Fatalf("Run() code = %d, want 0", code)
	}
	if got, want := service.request.Command, "feature enable"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := strings.Join(service.request.Args, ","), "trusted-interface"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if got, want := service.request.Flag("interface"), "tailscale0"; got != want {
		t.Fatalf("interface flag = %q, want %q", got, want)
	}
}

func TestRollbackTimeoutIsNotPublic(t *testing.T) {
	service := &recordingService{}
	var stderr strings.Builder
	runner := New(Options{Service: service, Stderr: &stderr})

	if code := runner.Run([]string{"apply", "--rollback-timeout", "1h"}); code == 0 {
		t.Fatal("removed rollback timeout flag was accepted")
	}
}

func TestFalseHelpAndVersionDoNotShortCircuit(t *testing.T) {
	service := &recordingService{}
	if code := New(Options{Service: service}).Run([]string{"--help=false", "--version=false", "status"}); code != 0 || service.request.Command != "status" {
		t.Fatalf("code=%d command=%q", code, service.request.Command)
	}
}

func TestVersionRejectsExtraArguments(t *testing.T) {
	if code := New(Options{}).Run([]string{"--version", "status"}); code == 0 {
		t.Fatal("--version accepted a command")
	}
}

func TestReportingFlagsRejectedForMutation(t *testing.T) {
	for _, args := range [][]string{{"open", "tcp", "443", "--output", "json"}, {"close", "tcp", "443", "--detail"}} {
		if code := New(Options{}).Run(args); code == 0 {
			t.Fatalf("reporting flags accepted for %v", args)
		}
	}
}

func TestUnknownCommandFails(t *testing.T) {
	var stderr strings.Builder
	runner := New(Options{Stderr: &stderr})

	if code := runner.Run([]string{"bogus"}); code == 0 {
		t.Fatalf("Run() code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), `unknown command "bogus"`) {
		t.Fatalf("stderr missing unknown command: %q", stderr.String())
	}
}

func TestUnknownFlagFails(t *testing.T) {
	var stderr strings.Builder
	runner := New(Options{Stderr: &stderr})

	if code := runner.Run([]string{"init", "--bogus"}); code == 0 {
		t.Fatalf("Run() code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "unknown flag --bogus") {
		t.Fatalf("stderr missing unknown flag: %q", stderr.String())
	}
}

func TestLeafPositionalArity(t *testing.T) {
	for _, args := range [][]string{{"open", "tcp"}, {"status", "extra"}, {"open", "tcp", "443", "extra"}} {
		var stderr strings.Builder
		if code := New(Options{Stderr: &stderr}).Run(args); code == 0 || !strings.Contains(stderr.String(), "positional argument") {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestRejectsDuplicateNonRepeatableFlag(t *testing.T) {
	var stderr strings.Builder
	args := []string{"init", "--wan-interface", "eth0", "--wan-interface=eth1"}
	if code := New(Options{Stderr: &stderr}).Run(args); code == 0 || !strings.Contains(stderr.String(), "may not be repeated") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRejectsExclusivePresetSources(t *testing.T) {
	var stderr strings.Builder
	args := []string{"init", "--preset", "abc", "--preset-file", "preset.json"}
	if code := New(Options{Stderr: &stderr}).Run(args); code == 0 || !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRootedConfigReadAndDDNSStatusBlackBox(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	outsideConfig := filepath.Join(outside, "config.yaml")
	cfg := config.Default()
	cfg.SSH.DDNSWhitelist.Enabled = true
	cfg.SSH.DDNSWhitelist.Hosts = []string{"home.example.com"}
	if err := config.SaveFile(outsideConfig, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	managedDir := filepath.Join(root, "etc/cnftctl")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideConfig, filepath.Join(managedDir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runner := New(Options{Stdout: &stdout, Stderr: &stderr, Service: app.NewService()})
	if code := runner.Run([]string{"--root", root, "config", "show"}); code == 0 || !strings.Contains(stderr.String(), "symlink") {
		t.Fatalf("escaped config code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	if err := os.Remove(filepath.Join(managedDir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFile(filepath.Join(managedDir, "config.yaml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"--root", root, "ddns", "status"}); code != 0 || !strings.Contains(stdout.String(), "not_applicable") {
		t.Fatalf("offline status code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
