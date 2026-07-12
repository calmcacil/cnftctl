package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBlackBoxVersionAndHelpOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("black-box executable test requires Unix executable semantics")
	}
	bin := filepath.Join(t.TempDir(), "cnftctl")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=9.8.7-test", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{{[]string{"--version"}, "cnftctl 9.8.7-test\n"}, {nil, "Usage:"}} {
		cmd := exec.Command(bin, tc.args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", tc.args, err, out)
		}
		if len(tc.args) > 0 && string(out) != tc.want {
			t.Fatalf("output=%q want=%q", out, tc.want)
		}
		if len(tc.args) == 0 && !strings.Contains(string(out), tc.want) {
			t.Fatalf("help missing %q:\n%s", tc.want, out)
		}
	}
}
