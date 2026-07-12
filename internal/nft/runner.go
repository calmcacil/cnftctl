package nft

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(context.Context, string, ...string) Result
}
type Result struct {
	Stdout, Stderr string
	Err            error
}
type SetReplacement struct {
	Set      string
	Elements []string
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
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: out.String(), Stderr: stderr.String(), Err: err}
}

func CheckDependencies(ctx context.Context, r Runner, names ...string) error {
	var missing []string
	for _, n := range names {
		if n != "" && !r.Run(ctx, "sh", "-c", "command -v "+shellQuote(n)).OK() {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required command(s): %v", missing)
	}
	return nil
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func ValidateFile(ctx context.Context, r Runner, path string) error {
	return fileCommand(ctx, r, true, path)
}
func LoadFile(ctx context.Context, r Runner, path string) error {
	return fileCommand(ctx, r, false, path)
}
func fileCommand(ctx context.Context, r Runner, check bool, path string) error {
	if path == "" {
		return errors.New("nft config path is required")
	}
	args := []string{"-f", path}
	verb := "load"
	if check {
		args = []string{"-c", "-f", path}
		verb = "validate"
	}
	res := r.Run(ctx, "nft", args...)
	if !res.OK() {
		return fmt.Errorf("%s nft config %s: %w", verb, path, res.Error())
	}
	return nil
}
func HasTable(ctx context.Context, r Runner, family, table string) (bool, error) {
	if family == "" || table == "" {
		return false, errors.New("nft family and table are required")
	}
	res := r.Run(ctx, "nft", "list", "table", family, table)
	if res.OK() {
		return true, nil
	}
	if isMissingTable(res.Stderr) {
		return false, nil
	}
	return false, fmt.Errorf("inspect nft table %s %s: %w", family, table, res.Error())
}
func ListTable(ctx context.Context, r Runner, family, table string) (string, bool, error) {
	if family == "" || table == "" {
		return "", false, errors.New("nft family and table are required")
	}
	res := r.Run(ctx, "nft", "list", "table", family, table)
	if res.OK() {
		return res.Stdout, true, nil
	}
	if isMissingTable(res.Stderr) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("inspect nft table %s %s: %w", family, table, res.Error())
}
func DeleteTable(ctx context.Context, r Runner, family, table string) error {
	if family == "" || table == "" {
		return errors.New("nft family and table are required")
	}
	f, err := os.CreateTemp("", "cnftctl-delete-*.nft")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err = f.WriteString("delete table " + family + " " + table + "\n"); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return LoadFile(ctx, r, path)
}

// ReplaceSet atomically flushes and repopulates one managed set in a single nft batch.
func ReplaceSet(ctx context.Context, r Runner, family, table, set string, elements []string) error {
	return ReplaceSets(ctx, r, family, table, []SetReplacement{{Set: set, Elements: elements}})
}

// ReplaceSets flushes and repopulates all supplied sets in one atomic nft batch.
func ReplaceSets(ctx context.Context, r Runner, family, table string, replacements []SetReplacement) error {
	for _, v := range []string{family, table} {
		if v == "" || strings.ContainsAny(v, " \t\r\n;{}") {
			return errors.New("invalid nft set identifier")
		}
	}
	var b strings.Builder
	for _, replacement := range replacements {
		if replacement.Set == "" || strings.ContainsAny(replacement.Set, " \t\r\n;{}") {
			return errors.New("invalid nft set identifier")
		}
		fmt.Fprintf(&b, "flush set %s %s %s\n", family, table, replacement.Set)
		if len(replacement.Elements) > 0 {
			fmt.Fprintf(&b, "add element %s %s %s { %s }\n", family, table, replacement.Set, strings.Join(replacement.Elements, ", "))
		}
	}
	f, err := os.CreateTemp("", "cnftctl-set-*.nft")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err = f.WriteString(b.String()); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return LoadFile(ctx, r, path)
}
func isMissingTable(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "no such file or directory") || strings.Contains(s, "does not exist") || strings.Contains(s, "no such table")
}
