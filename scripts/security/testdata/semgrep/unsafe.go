package testdata

import (
	"context"
	"os/exec"
)

func RunDynamic(ctx context.Context, name string) error {
	return exec.CommandContext(ctx, name).Run()
}

