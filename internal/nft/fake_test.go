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
		{Name: "nft", Args: []string{"-c", "-I", "/tmp", "-f", "/tmp/nftables.conf"}},
		{Name: "nft", Args: []string{"-I", "/tmp", "-f", "/tmp/nftables.conf"}},
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

func TestHasTableDistinguishesMissingFromErrors(t *testing.T) {
	tests := []struct {
		name    string
		result  Result
		present bool
		wantErr bool
	}{
		{name: "present", result: Result{}, present: true},
		{name: "missing", result: Result{Stderr: "Error: No such file or directory", Err: errors.New("exit 1")}},
		{name: "permission", result: Result{Stderr: "Operation not permitted", Err: errors.New("exit 1")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{results: []Result{tt.result}}
			present, err := HasTable(context.Background(), r, "inet", "hostfw")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if present != tt.present {
				t.Fatalf("present = %t, want %t", present, tt.present)
			}
		})
	}
}

func TestDeleteTableUsesTargetedBatch(t *testing.T) {
	r := &fakeRunner{}
	if err := DeleteTable(context.Background(), r, "inet", "hostfw"); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0].Args[:1], []string{"-I"}) {
		t.Fatalf("calls = %#v", r.calls)
	}
}

func TestReplaceSetRejectsUnsafeIdentifier(t *testing.T) {
	if err := ReplaceSet(context.Background(), &fakeRunner{}, "inet", "hostfw", "x; flush ruleset", nil); err == nil {
		t.Fatal("expected unsafe identifier error")
	}
}

func TestReplaceSetsUsesOneBatch(t *testing.T) {
	r := &fakeRunner{}
	err := ReplaceSets(context.Background(), r, "inet", "hostfw", []SetReplacement{{Set: "v4", Elements: []string{"203.0.113.1"}}, {Set: "v6", Elements: []string{"2001:db8::/56"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || len(r.calls[0].Args) < 2 || r.calls[0].Args[len(r.calls[0].Args)-2] != "-f" {
		t.Fatalf("calls = %#v", r.calls)
	}
}
