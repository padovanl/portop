// Package hostcmd runs system inspection tools with a bounded runtime and
// locale-independent output. Arguments are passed directly, without a shell.
package hostcmd

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func Output(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return out, err
}
