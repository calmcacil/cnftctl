package nft

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type call struct {
	Name string
	Args []string
}

type fakeRunner struct {
	results []Result
	calls   []call
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) Result {
	f.calls = append(f.calls, call{Name: name, Args: append([]string(nil), args...)})
	if len(f.results) == 0 {
		return Result{}
	}
	res := f.results[0]
	f.results = f.results[1:]
	return res
}

func TestValidateAndLoadUseNft(t *testing.T) {
	r := &fakeRunner{}
	if err := ValidateFile(context.Background(), r, "/tmp/nftables.conf"); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(context.Background(), r, "/tmp/nftables.conf"); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{Name: "nft", Args: []string{"-c", "-f", "/tmp/nftables.conf"}},
		{Name: "nft", Args: []string{"-f", "/tmp/nftables.conf"}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestValidateReportsRunnerError(t *testing.T) {
	r := &fakeRunner{results: []Result{{Stderr: "syntax error", Err: errors.New("exit 1")}}}
	if err := ValidateFile(context.Background(), r, "/tmp/bad.conf"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckDependencies(t *testing.T) {
	r := &fakeRunner{results: []Result{{}, {Err: errors.New("not found")}}}
	if err := CheckDependencies(context.Background(), r, "nft", "systemctl"); err == nil {
		t.Fatal("expected missing dependency error")
	}
}
