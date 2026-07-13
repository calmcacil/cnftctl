package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calmcacil/cnftctl/internal/apply"
	"github.com/calmcacil/cnftctl/internal/config"
	"github.com/calmcacil/cnftctl/internal/ddns"
	"github.com/calmcacil/cnftctl/internal/nft"
	"github.com/calmcacil/cnftctl/internal/render"
)

type appCall struct {
	name string
	args []string
}

type appFakeRunner struct {
	calls   []appCall
	results []nftResult
}

type nftResult struct {
	stdout string
	stderr string
	err    error
}

type appFakeResolver struct {
	calls int
}

func (r *appFakeResolver) LookupA(context.Context, string) ([]netip.Addr, error) {
	r.calls++
	return nil, errors.New("unexpected resolver call")
}

func (r *appFakeResolver) LookupAAAA(context.Context, string) ([]netip.Addr, error) {
	r.calls++
	return nil, errors.New("unexpected resolver call")
}

func (f *appFakeRunner) Run(_ context.Context, name string, args ...string) nft.Result {
	f.calls = append(f.calls, appCall{name: name, args: append([]string(nil), args...)})
	if len(f.results) == 0 {
		return nft.Result{}
	}
	res := f.results[0]
	f.results = f.results[1:]
	return nft.Result{Stdout: res.stdout, Stderr: res.stderr, Err: res.err}
}

func TestServiceInitDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{
		Command: "init",
		Flags: map[string][]string{
			"root":          {root},
			"dry-run":       {"true"},
			"wan-interface": {"eth0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/cnftctl/config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config: %v", err)
	}
	if !strings.Contains(stdout.String(), defaultConfigPath) || strings.Contains(stdout.String(), render.NftablesConfPath) {
		t.Fatalf("dry-run must preview config only: %s", stdout.String())
	}
}

func TestServiceInitRequiresYesForWrites(t *testing.T) {
	root := t.TempDir()
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, CommandRequest{
		Command: "init",
		Flags: map[string][]string{
			"root":          {root},
			"wan-interface": {"eth0"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "etc/cnftctl/config.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("init without --yes wrote config: %v", statErr)
	}

	err = svc.Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, CommandRequest{
		Command: "init",
		Flags: map[string][]string{
			"root":          {root},
			"wan-interface": {"eth0"},
			"yes":           {"true"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "etc/cnftctl/config.yaml")); statErr != nil {
		t.Fatalf("init with --yes did not write config: %v", statErr)
	}
}

func TestServiceOpenUpdatesConfigOnly(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	var stdout, stderr bytes.Buffer
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "open",
		Args:    []string{"tcp", "443"},
		Flags: map[string][]string{
			"root":    {root},
			"comment": {"HTTPS"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.OpenPorts) != 1 || cfg.OpenPorts[0].Protocol != "tcp" || cfg.OpenPorts[0].Port != 443 {
		t.Fatalf("unexpected open ports: %#v", cfg.OpenPorts)
	}
	if _, err := os.Stat(filepath.Join(root, strings.TrimPrefix(render.OpenPortsPath, "/"))); !os.IsNotExist(err) {
		t.Fatalf("config mutation wrote rendered file: %v", err)
	}
	if !strings.Contains(stdout.String(), "run cnftctl apply") {
		t.Fatalf("missing apply guidance: %s", stdout.String())
	}
}

func TestServiceCloseStrictRejectsMissingPort(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	var stdout, stderr bytes.Buffer
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "close",
		Args:    []string{"tcp", "443"},
		Flags: map[string][]string{
			"root":   {root},
			"strict": {"true"},
		},
	})
	if err == nil {
		t.Fatal("expected strict close to reject a missing port")
	}
	if !strings.Contains(err.Error(), "is not open") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceWhitelistAddRemoveAndList(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	svc := NewService()

	var stdout, stderr bytes.Buffer
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "whitelist add",
		Args:    []string{"203.0.113.10"},
		Flags:   map[string][]string{"root": {root}, "comment": {"current office"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "whitelist add 203.0.113.10/32 changed=true") {
		t.Fatalf("unexpected add output: %s", stdout.String())
	}

	stdout.Reset()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{Command: "whitelist list", Flags: map[string][]string{"root": {root}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "203.0.113.10/32" {
		t.Fatalf("unexpected whitelist list: %q", stdout.String())
	}
	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SSH.StaticWhitelist.IPv4) != 1 || cfg.SSH.StaticWhitelist.IPv4[0].Comment != "current office" {
		t.Fatalf("whitelist comment was not preserved: %#v", cfg.SSH.StaticWhitelist.IPv4)
	}

	stdout.Reset()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "whitelist remove",
		Args:    []string{"203.0.113.10"},
		Flags:   map[string][]string{"root": {root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "changed=true") {
		t.Fatalf("unexpected remove output: %s", stdout.String())
	}
	cfg, err = config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SSH.StaticWhitelist.IPv4) != 0 || len(cfg.SSH.StaticWhitelist.IPv6) != 0 {
		t.Fatalf("expected empty whitelist after remove: %#v", cfg.SSH.StaticWhitelist)
	}
}

func TestServiceDDNSCommandsUpdateConfig(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	svc := NewService()
	io := IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}

	commands := []CommandRequest{
		{Command: "ddns add", Args: []string{"Home.Example.Com."}, Flags: map[string][]string{"root": {root}}},
		{Command: "ddns enable", Flags: map[string][]string{"root": {root}}},
		{Command: "ddns set-ipv6-prefix-len", Args: []string{"64"}, Flags: map[string][]string{"root": {root}}},
	}
	for _, req := range commands {
		if err := svc.Run(context.Background(), io, req); err != nil {
			t.Fatalf("%s failed: %v", req.Command, err)
		}
	}

	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SSH.DDNSWhitelist.Enabled || cfg.SSH.DDNSWhitelist.IPv6PrefixLen != 64 {
		t.Fatalf("unexpected DDNS config: %#v", cfg.SSH.DDNSWhitelist)
	}
	if len(cfg.SSH.DDNSWhitelist.Hosts) != 1 || cfg.SSH.DDNSWhitelist.Hosts[0] != "home.example.com" {
		t.Fatalf("unexpected DDNS hosts: %#v", cfg.SSH.DDNSWhitelist.Hosts)
	}

	var stdout bytes.Buffer
	if err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "ddns status", Flags: map[string][]string{"root": {root}}}); err != nil {
		t.Fatalf("expected offline status report, got %v", err)
	}
	for _, want := range []string{"ddns.runtime", "not_applicable", "alternate root"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status output missing %q: %s", want, stdout.String())
		}
	}
}

func TestServiceDDNSStatusShowsRuntimeEntries(t *testing.T) {
	cfg := config.Default()
	cfg.SSH.DDNSWhitelist.Enabled = true
	cfg.SSH.DDNSWhitelist.Hosts = []string{"home.example.com"}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := config.SaveFile(path, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &appFakeRunner{results: []nftResult{
		{stdout: `{"nftables":[{"set":{"elem":["203.0.113.10"]}}]}`},
		{stdout: `{"nftables":[{"set":{"elem":[{"prefix":{"addr":"2001:db8::","len":56}}]}}]}`},
	}}
	svc := realService{runner: runner}
	var stdout bytes.Buffer
	if err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "ddns status", Flags: map[string][]string{"config": {path}, "detail": {"true"}}}); !IsHealthError(err) {
		t.Fatalf("expected completed unhealthy status, got %v", err)
	}
	for _, want := range []string{"runtime_ipv4: [203.0.113.10]", "runtime_ipv6: [2001:db8::/56]"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status output missing %q: %s", want, stdout.String())
		}
	}
}

func TestAlternateRootDDNSStatusDoesNotUseHostRuntime(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.SSH.DDNSWhitelist.Enabled = true
	cfg.SSH.DDNSWhitelist.Hosts = []string{"home.example.com"}
	writeTestConfig(t, root, cfg)
	runner, resolver := &appFakeRunner{}, &appFakeResolver{}
	var stdout bytes.Buffer
	err := (realService{runner: runner, resolver: resolver}).Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "ddns status", Flags: map[string][]string{"root": {root}}})
	if err != nil || len(runner.calls) != 0 || resolver.calls != 0 {
		t.Fatalf("err=%v nft calls=%v resolver calls=%d", err, runner.calls, resolver.calls)
	}
	if !strings.Contains(stdout.String(), "not_applicable") {
		t.Fatalf("missing offline result: %s", stdout.String())
	}
}

func TestAlternateRootConfigReadsRequireManagedContainment(t *testing.T) {
	commands := []CommandRequest{
		{Command: "config show"},
		{Command: "validate"},
		{Command: "plan"},
		{Command: "apply", Flags: map[string][]string{"dry-run": {"true"}}},
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root, outside string) string
	}{
		{"relative", func(*testing.T, string, string) string { return "etc/cnftctl/config.yaml" }},
		{"dotdot", func(*testing.T, string, string) string { return "/etc/../config.yaml" }},
		{"ancestor_symlink", func(t *testing.T, root, outside string) string {
			if err := os.Symlink(outside, filepath.Join(root, "etc")); err != nil {
				t.Fatal(err)
			}
			return "/etc/config.yaml"
		}},
		{"final_symlink", func(t *testing.T, root, outside string) string {
			dir := filepath.Join(root, "etc/cnftctl")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := config.SaveFile(filepath.Join(outside, "config.yaml"), config.Default(), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "config.yaml"), filepath.Join(dir, "config.yaml")); err != nil {
				t.Fatal(err)
			}
			return "/etc/cnftctl/config.yaml"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, command := range commands {
				root, outside := t.TempDir(), t.TempDir()
				writeTestConfig(t, outside, config.Default())
				path := tc.setup(t, root, outside)
				req := command
				if req.Flags == nil {
					req.Flags = map[string][]string{}
				}
				req.Flags["root"], req.Flags["config"] = []string{root}, []string{path}
				err := (realService{runner: &appFakeRunner{}}).Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, req)
				if err == nil {
					t.Fatalf("command %q accepted %s config path", req.Command, tc.name)
				}
			}
		})
	}
}

type batchRunner struct {
	batch string
	calls int
	err   error
}

func (r *batchRunner) Run(_ context.Context, name string, args ...string) nft.Result {
	r.calls++
	if name != "nft" || len(args) < 2 || args[len(args)-2] != "-f" {
		return nft.Result{Err: errors.New("unexpected command")}
	}
	data, err := os.ReadFile(args[len(args)-1])
	r.batch = string(data)
	if err != nil {
		return nft.Result{Err: err}
	}
	return nft.Result{Err: r.err}
}

func TestNFTRuntimeJointBatchFailureIsReportedWithoutSecondMutation(t *testing.T) {
	runner := &batchRunner{err: errors.New("atomic nft batch rejected")}
	err := (nftRuntime{runner: runner}).Refresh(context.Background(), []netip.Addr{netip.MustParseAddr("203.0.113.10")}, []netip.Prefix{netip.MustParsePrefix("2001:db8::/56")}, time.Hour)
	if err == nil || !strings.Contains(err.Error(), "atomic nft batch rejected") {
		t.Fatalf("error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("nft calls=%d, want one atomic attempt", runner.calls)
	}
	if !strings.Contains(runner.batch, "ddns_whitelist_v4") || !strings.Contains(runner.batch, "ddns_whitelist_v6") {
		t.Fatalf("joint batch missing a family:\n%s", runner.batch)
	}
}

func TestReportJSONContractAndHealthStates(t *testing.T) {
	states := []struct {
		state     State
		unhealthy bool
	}{{StateOK, false}, {StateNotApplicable, false}, {StateAbsent, true}, {StatePending, true}, {StateDegraded, true}, {StateFailed, true}, {StateUnknown, true}, {StateUnsupported, true}}
	for _, tc := range states {
		t.Run(string(tc.state), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			report := newReport("status", []Check{{ID: "contract", State: tc.state, Summary: "stable"}}, map[string]any{"value": 1})
			err := finishReport(IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{Flags: map[string][]string{"output": {"json"}}}, report)
			if IsHealthError(err) != tc.unhealthy {
				t.Fatalf("health error=%v for %s", err, tc.state)
			}
			if stderr.Len() != 0 {
				t.Fatalf("JSON contract wrote stderr: %q", stderr.String())
			}
			var decoded map[string]any
			wantOverall := string(tc.state)
			if tc.state == StateNotApplicable {
				wantOverall = string(StateOK)
			}
			if json.Unmarshal(stdout.Bytes(), &decoded) != nil || decoded["schema"] != ReportSchemaVersion || decoded["command"] != "status" || decoded["state"] != wantOverall {
				t.Fatalf("invalid JSON contract: %s", stdout.String())
			}
			checks := decoded["checks"].([]any)
			if checks[0].(map[string]any)["state"] != string(tc.state) {
				t.Fatalf("check state lost in JSON contract: %s", stdout.String())
			}
		})
	}
}

func TestReportJSONHidesDetailUnlessRequested(t *testing.T) {
	report := newReport("status", []Check{{ID: "secret", State: StateOK, Summary: "ok", Detail: map[string]any{"token": "sensitive"}}}, nil)
	for _, detail := range []bool{false, true} {
		var out bytes.Buffer
		req := CommandRequest{Flags: map[string][]string{"output": {"json"}}}
		if detail {
			req.Flags["detail"] = []string{"true"}
		}
		if err := finishReport(IO{Stdout: &out}, req, report); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "sensitive") != detail {
			t.Fatalf("detail=%t output=%s", detail, out.String())
		}
	}
}

func TestAlternateRootRejectsOnlineCommandsWithoutRunnerCalls(t *testing.T) {
	for _, command := range []string{"apply", "confirm", "rollback", "reconcile", "ddns refresh", "ddns timer status"} {
		runner := &appFakeRunner{}
		err := (realService{runner: runner}).Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, CommandRequest{Command: command, Flags: map[string][]string{"root": {t.TempDir()}}})
		if err == nil || len(runner.calls) != 0 {
			t.Fatalf("command=%q err=%v calls=%v", command, err, runner.calls)
		}
	}
}

func TestOpenSSHApplyDoesNotCreateOverrideMetadata(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	var stdout bytes.Buffer
	err := (realService{runner: &appFakeRunner{}}).Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{
		Command:     "apply",
		Flags:       map[string][]string{"root": {root}, "dry-run": {"true"}},
		Environment: map[string]string{"SSH_CONNECTION": "203.0.113.10 12345 198.51.100.10 22"},
	})
	if err != nil {
		t.Fatalf("open SSH dry-run apply failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "dry-run: no files written and nftables not loaded") {
		t.Fatalf("dry-run output missing plan: %q", stdout.String())
	}
}

func TestManagedWriteRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "etc")); err != nil {
		t.Fatal(err)
	}
	req := CommandRequest{Flags: map[string][]string{"root": {root}, "config": {"/etc/config.yaml"}}}
	if err := writeConfig(req, config.Default()); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("outside path was written: %v", err)
	}
}

func TestParseOSReleaseExactValues(t *testing.T) {
	values, err := parseOSRelease([]byte("ID=debian\nVERSION_ID=\"13\"\nVERSION_CODENAME=trixie\n"))
	if err != nil || values["ID"] != "debian" || values["VERSION_ID"] != "13" || values["VERSION_CODENAME"] != "trixie" {
		t.Fatalf("values=%v err=%v", values, err)
	}
	values, err = parseOSRelease([]byte("ID=debianized\nVERSION_ID=\"13\"\n"))
	if err != nil || values["ID"] == "debian" {
		t.Fatalf("substring was accepted: %v %v", values, err)
	}
	if _, err := parseOSRelease([]byte("ID=debian\t13\n")); err == nil {
		t.Fatal("unquoted tab was accepted")
	}
}

func FuzzNftSetJSON(f *testing.F) {
	f.Add(`{"nftables":[{"set":{"elem":["203.0.113.1",{"prefix":{"addr":"2001:db8::","len":56}}]}}]}`)
	f.Add(`{"nftables":[{"set":{"elem":[{"elem":{"val":"203.0.113.10","expires":3599}}]}}]}`)
	f.Add(`{"nftables":[]}`)
	f.Fuzz(func(t *testing.T, data string) { _, _ = parseNftSetElements(data) })
}

func TestParseNftSetElementsWithTimeoutMetadata(t *testing.T) {
	data := `{"nftables":[{"set":{"elem":[{"elem":{"val":"203.0.113.10","expires":3599}},{"elem":{"val":{"prefix":{"addr":"2001:db8:1234:5600::","len":56}},"expires":3599}}]}}]}`
	values, err := parseNftSetElements(data)
	if err != nil || strings.Join(values, ",") != "203.0.113.10,2001:db8:1234:5600::/56" {
		t.Fatalf("values=%v err=%v", values, err)
	}
}

func TestNFTRuntimeRefreshUsesOneJointBatch(t *testing.T) {
	runner := &batchRunner{}
	err := (nftRuntime{runner: runner}).Refresh(context.Background(), []netip.Addr{netip.MustParseAddr("203.0.113.10")}, []netip.Prefix{netip.MustParsePrefix("2001:db8::/56")}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("nft calls=%d, want 1", runner.calls)
	}
	for _, want := range []string{"flush set inet hostfw ddns_whitelist_v4", "flush set inet hostfw ddns_whitelist_v6", "203.0.113.10 timeout 1h", "2001:db8::/56 timeout 1h"} {
		if !strings.Contains(runner.batch, want) {
			t.Fatalf("batch missing %q:\n%s", want, runner.batch)
		}
	}
}

func TestServiceFeatureCommands(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	svc := NewService()
	var stdout, stderr bytes.Buffer

	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "feature enable",
		Args:    []string{"docker"},
		Flags:   map[string][]string{"root": {root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Docker-published ports remain blocked") {
		t.Fatalf("missing docker warning: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "feature enable",
		Args:    []string{"trusted-interface"},
		Flags: map[string][]string{
			"root":      {root},
			"interface": {"tailscale0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "fully trusted") {
		t.Fatalf("missing trusted interface warning: %s", stderr.String())
	}

	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Docker.Enabled || !cfg.TrustedInterfaces.Enabled || len(cfg.TrustedInterfaces.Interfaces) != 1 || cfg.TrustedInterfaces.Interfaces[0] != "tailscale0" {
		t.Fatalf("unexpected feature config: %#v", cfg)
	}
}

func TestServiceDockerBackendWriteRequiresYes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/docker/daemon.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"log-level":"info"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "docker backend write", Flags: map[string][]string{"root": {root}}})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{Command: "docker backend write", Flags: map[string][]string{"root": {root}, "yes": {"true"}}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"firewall-backend": "nftables"`) {
		t.Fatalf("daemon.json missing backend: %s", data)
	}
	if !strings.Contains(stderr.String(), "must not restart Docker") || !strings.Contains(stdout.String(), "restart Docker manually") {
		t.Fatalf("missing docker safety output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestServiceDockerBackendRejectsInvalidBackendType(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/docker/daemon.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"firewall-backend":true}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	err := NewService().Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, CommandRequest{
		Command: "docker backend write",
		Flags:   map[string][]string{"root": {root}, "yes": {"true"}},
	})
	if err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Fatalf("expected invalid backend type error, got %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatalf("invalid daemon config was changed: %q", after)
	}
}

func TestValidateDockerDaemonConfigUsesInstalledDaemon(t *testing.T) {
	runner := &appFakeRunner{}
	if err := validateDockerDaemonConfig(context.Background(), runner, []byte(`{"firewall-backend":"nftables"}`)); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != "dockerd" || len(runner.calls[0].args) != 2 || runner.calls[0].args[0] != "--validate" || !strings.HasPrefix(runner.calls[0].args[1], "--config-file=") {
		t.Fatalf("unexpected daemon validation call: %#v", runner.calls)
	}
	path := strings.TrimPrefix(runner.calls[0].args[1], "--config-file=")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("staged daemon configuration was not removed: %v", err)
	}
}

func TestValidateDockerDaemonConfigRejectsUnsupportedBackend(t *testing.T) {
	runner := &appFakeRunner{results: []nftResult{{stderr: "unknown option: firewall-backend", err: errors.New("exit status 1")}}}
	err := validateDockerDaemonConfig(context.Background(), runner, []byte(`{"firewall-backend":"nftables"}`))
	if err == nil || !strings.Contains(err.Error(), "rejected proposed configuration") || !strings.Contains(err.Error(), "no file was written") || !strings.Contains(err.Error(), "firewall-backend") {
		t.Fatalf("expected actionable compatibility error, got %v", err)
	}
}

func TestServiceAdoptReferenceRequiresYes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc/nftables.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/nftables.d/open-ports.nft"), []byte("tcp . 443,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/nftables.d/whitelist.nft"), []byte("define whitelist_v4 = { 203.0.113.10 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "adopt reference", Flags: map[string][]string{"root": {root}}})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
	var stdout bytes.Buffer
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "adopt reference", Flags: map[string][]string{"root": {root}, "yes": {"true"}}})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.OpenPorts) != 1 || cfg.OpenPorts[0].Port != 443 || len(cfg.SSH.StaticWhitelist.IPv4) != 1 {
		t.Fatalf("unexpected adopted config: %#v", cfg)
	}
	if !strings.Contains(stdout.String(), "run cnftctl apply") {
		t.Fatalf("missing apply guidance: %s", stdout.String())
	}
}

func TestServicePresetExplain(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.OpenPorts = append(cfg.OpenPorts, config.OpenPort{Protocol: "tcp", Port: 443})
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	presetPath := filepath.Join(root, "preset.json")
	if err := os.WriteFile(presetPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	svc := NewService()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "preset explain", Args: []string{presetPath}, Flags: map[string][]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "risk warnings:") {
		t.Fatalf("expected risk warnings: %s", stdout.String())
	}
}

func TestPlanJSONUsesVersionedReportAndPendingHealth(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	var stdout bytes.Buffer
	err := NewService().Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "plan", Flags: map[string][]string{"root": {root}, "output": {"json"}}})
	if !IsHealthError(err) {
		t.Fatalf("expected pending health error, got %v", err)
	}
	var report Report
	if decodeErr := json.Unmarshal(stdout.Bytes(), &report); decodeErr != nil {
		t.Fatalf("stdout is not pure JSON: %v\n%s", decodeErr, stdout.String())
	}
	if report.Schema != ReportSchemaVersion || report.Command != "plan" || report.State != StatePending {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestDDNSCandidateFinalizationUsesOneGenerationIdentity(t *testing.T) {
	cfg := config.Default()
	cfg.SSH.DDNSWhitelist.Enabled = true
	cfg.SSH.DDNSWhitelist.Hosts = []string{"router.example.com"}
	_, files, err := generationFiles("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	files, err = embedDDNSCandidate(files, ddns.RefreshResult{IPv4: []netip.Addr{netip.MustParseAddr("203.0.113.10")}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	generation, files, err := finalizeGeneration(files, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !strings.Contains(file.Path, generation) {
			t.Fatalf("file path %q does not use generation %s", file.Path, generation)
		}
		if filepath.Base(file.Path) == "firewall.nft" {
			text := string(file.Data)
			if !strings.Contains(text, apply.OwnershipMarker+":generation:"+generation) || !strings.Contains(text, `include "whitelist.nft"`) {
				t.Fatalf("final firewall identity mismatch:\n%s", text)
			}
		}
	}
	derived, _, err := apply.FinalizeFiles(files, true)
	if err != nil || derived != generation {
		t.Fatalf("Apply derivation=%q err=%v, want %q", derived, err, generation)
	}
}

func TestDDNSIntentGenerationIgnoresInitialResolvedElements(t *testing.T) {
	cfg := config.Default()
	cfg.SSH.DDNSWhitelist.Enabled = true
	cfg.SSH.DDNSWhitelist.Hosts = []string{"router.example.com"}
	desired, files, err := generationFiles("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	files, err = embedDDNSCandidate(files, ddns.RefreshResult{
		IPv4: []netip.Addr{netip.MustParseAddr("203.0.113.10")},
		IPv6: []netip.Prefix{netip.MustParsePrefix("2001:db8:1234:5600::/56")},
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, files, err = finalizeGeneration(files, true)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := ddnsIntentGeneration(files)
	if err != nil || intent != desired {
		t.Fatalf("intent=%q err=%v, want %q", intent, err, desired)
	}
}

func writeTestConfig(t *testing.T, root string, cfg config.Config) {
	t.Helper()
	path := filepath.Join(root, "etc/cnftctl/config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFile(path, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
}
