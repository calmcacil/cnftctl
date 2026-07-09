package app

import (
	"context"
	"fmt"
)

// StubService keeps all planned commands non-privileged and testable until the
// implementation packages are connected.
type StubService struct{}

func (StubService) Run(_ context.Context, io IO, request CommandRequest) error {
	_, err := fmt.Fprintf(io.Stdout, "%s: not implemented yet\n", request.Command)
	return err
}
