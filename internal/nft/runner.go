package nft

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes external commands. Tests can replace it with a fake so no
// root privileges or live nftables installation is needed.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) Result
}

type Result struct {
	Stdout string
	Stderr string
	Err    error
}

func (r Result) OK() bool { return r.Err == nil }

func (r Result) Error() error {
	if r.Err == nil {
		return nil
	}
	if r.Stderr != "" {
		return fmt.Errorf("%w: %s", r.Err, r.Stderr)
	}
	return r.Err
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) Result {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

func CheckDependencies(ctx context.Context, runner Runner, names ...string) error {
	var missing []string
	for _, name := range names {
		if name == "" {
			continue
		}
		if res := runner.Run(ctx, "sh", "-c", "command -v "+shellQuote(name)); !res.OK() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required command(s): %v", missing)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func ValidateFile(ctx context.Context, runner Runner, path string) error {
	if path == "" {
		return errors.New("nft config path is required")
	}
	res := runner.Run(ctx, "nft", "-c", "-f", path)
	if !res.OK() {
		return fmt.Errorf("validate nft config %s: %w", path, res.Error())
	}
	return nil
}

func LoadFile(ctx context.Context, runner Runner, path string) error {
	if path == "" {
		return errors.New("nft config path is required")
	}
	res := runner.Run(ctx, "nft", "-f", path)
	if !res.OK() {
		return fmt.Errorf("load nft config %s: %w", path, res.Error())
	}
	return nil
}

func HasTable(ctx context.Context, runner Runner, family, table string) (bool, error) {
	if family == "" || table == "" {
		return false, errors.New("nft family and table are required")
	}
	res := runner.Run(ctx, "nft", "list", "table", family, table)
	if res.OK() {
		return true, nil
	}
	return false, nil
}
