package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// coreVersionCheck is the bounded fallback for firmware without timeout(1).
// It is dispatched before config loading, logging or service initialization.
func coreVersionCheck(binary string, budget time.Duration, output io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	buf := &versionOutput{limit: 32 * 1024}
	cmd := exec.CommandContext(ctx, binary, "-v")
	cmd.Stdout, cmd.Stderr = buf, buf
	cmd.WaitDelay = 250 * time.Millisecond
	err := cmd.Run()
	if _, writeErr := output.Write(buf.Bytes()); writeErr != nil {
		return 126
	}
	if ctx.Err() != nil {
		fmt.Fprintln(output, "core version check exceeded deadline")
		return 124
	}
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return exit.ExitCode()
	}
	fmt.Fprintln(output, "cannot execute selected core:", err)
	return 126
}

// Discard excessive version output without allocating an unbounded buffer.
// os/exec serializes writes when stdout and stderr share the same writer.
type versionOutput struct {
	// Do not embed bytes.Buffer: its promoted ReadFrom lets io.Copy bypass Write.
	buffer bytes.Buffer
	limit  int
}

func (b *versionOutput) Bytes() []byte {
	return b.buffer.Bytes()
}

func (b *versionOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := b.limit - b.buffer.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buffer.Write(p)
	}
	return n, nil
}
