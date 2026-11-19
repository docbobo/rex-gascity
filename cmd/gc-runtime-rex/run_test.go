package main

import (
	"context"
	"io"
)

// run dispatches one RPP op with a background context and returns the process
// exit code. It is the test entry point for callers that do not carry a signal
// context.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, d deps) int {
	return runWithContext(context.Background(), args, stdin, stdout, stderr, d)
}
