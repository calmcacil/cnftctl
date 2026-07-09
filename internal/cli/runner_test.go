package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/calmcacil/cnftctl/internal/app"
)

type recordingService struct {
	request app.CommandRequest
}

func (s *recordingService) Run(_ context.Context, _ app.IO, request app.CommandRequest) error {
	s.request = request
	return nil
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

func TestApplyDefaultRollbackTimeout(t *testing.T) {
	service := &recordingService{}
	runner := New(Options{Service: service})

	if code := runner.Run([]string{"apply"}); code != 0 {
		t.Fatalf("Run() code = %d, want 0", code)
	}
	if got, want := service.request.Flag("rollback-timeout"), "120s"; got != want {
		t.Fatalf("rollback-timeout = %q, want %q", got, want)
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
