package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const ReportSchemaVersion = "cnftctl.report.v1"

type State string

const (
	StateOK            State = "ok"
	StateAbsent        State = "absent"
	StatePending       State = "pending"
	StateDegraded      State = "degraded"
	StateFailed        State = "failed"
	StateUnknown       State = "unknown"
	StateUnsupported   State = "unsupported"
	StateNotApplicable State = "not_applicable"
)

type Check struct {
	ID      string         `json:"id"`
	State   State          `json:"state"`
	Summary string         `json:"summary"`
	Code    string         `json:"code,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

type Report struct {
	Schema  string         `json:"schema"`
	Command string         `json:"command"`
	State   State          `json:"state"`
	Checks  []Check        `json:"checks"`
	Data    map[string]any `json:"data,omitempty"`
}

// HealthError means an inspection completed and produced usable output, but its
// conservative overall state is not healthy. The CLI maps it to exit status 1.
type HealthError struct{ State State }

func (e HealthError) Error() string { return "completed with state " + string(e.State) }

func IsHealthError(err error) bool {
	var target HealthError
	return errors.As(err, &target)
}

func newReport(command string, checks []Check, data map[string]any) Report {
	r := Report{Schema: ReportSchemaVersion, Command: command, Checks: checks, Data: data}
	r.State = overallState(checks)
	return r
}

func overallState(checks []Check) State {
	state := StateOK
	rank := map[State]int{StateOK: 0, StateNotApplicable: 0, StateAbsent: 2, StatePending: 3, StateDegraded: 4, StateUnsupported: 5, StateUnknown: 6, StateFailed: 7}
	for _, check := range checks {
		if rank[check.State] > rank[state] {
			state = check.State
		}
	}
	return state
}

func writeReport(w io.Writer, output string, detail bool, report Report) error {
	if output == "" {
		output = "text"
	}
	switch output {
	case "json":
		if !detail {
			report.Checks = append([]Check(nil), report.Checks...)
			for i := range report.Checks {
				report.Checks[i].Detail = nil
			}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "text":
		fmt.Fprintf(w, "%s: %s\n", report.Command, report.State)
		for _, check := range report.Checks {
			fmt.Fprintf(w, "%-30s %-14s %s", check.ID, check.State, check.Summary)
			if check.Code != "" {
				fmt.Fprintf(w, " [%s]", check.Code)
			}
			fmt.Fprintln(w)
			if detail && len(check.Detail) > 0 {
				keys := make([]string, 0, len(check.Detail))
				for key := range check.Detail {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					fmt.Fprintf(w, "  %s: %v\n", key, check.Detail[key])
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid_output: --output must be text or json, got %q", strings.TrimSpace(output))
	}
}

func finishReport(io IO, req CommandRequest, report Report) error {
	if err := writeReport(io.Stdout, req.Flag("output"), req.BoolFlag("detail"), report); err != nil {
		return err
	}
	if report.State != StateOK && report.State != StateNotApplicable {
		return HealthError{State: report.State}
	}
	return nil
}
