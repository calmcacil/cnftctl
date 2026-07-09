package app

import (
	"context"
	"io"
	"time"
)

// IO carries command streams through the application boundary.
type IO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// CommandRequest is the shared, minimal contract between CLI wiring and the
// packages that will later implement config, render, apply, and feature logic.
type CommandRequest struct {
	Command string
	Args    []string
	Flags   map[string][]string
}

func (r CommandRequest) Flag(name string) string {
	values := r.Flags[name]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func (r CommandRequest) FlagValues(name string) []string {
	values := r.Flags[name]
	return append([]string(nil), values...)
}

func (r CommandRequest) BoolFlag(name string) bool {
	return r.Flag(name) == "true"
}

func (r CommandRequest) DurationFlag(name string) (time.Duration, error) {
	value := r.Flag(name)
	if value == "" {
		return 0, nil
	}
	return time.ParseDuration(value)
}

// Service is intentionally small so this CLI foundation can compile before the
// feature packages exist. Future packages can satisfy this interface directly or
// through an adapter in internal/app.
type Service interface {
	Run(ctx context.Context, io IO, request CommandRequest) error
}
