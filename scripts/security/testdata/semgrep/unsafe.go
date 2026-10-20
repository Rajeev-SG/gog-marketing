package testdata

import (
	"context"
	"net/http"
	"os/exec"
)

func RunHTTPCommand(r *http.Request) error {
	name := r.FormValue("binary")
	// ruleid: go.http-command-injection
	return exec.CommandContext(r.Context(), name).Run()
}

func RunHTTPShell(r *http.Request) error {
	command := r.URL.Query().Get("command")
	// ruleid: go.http-command-injection
	return exec.Command("sh", "-c", command).Run()
}

func RunTrustedConfiguration(ctx context.Context, binary string) error {
	// ok: go.http-command-injection
	return exec.CommandContext(ctx, binary).Run()
}

func PassArgumentWithoutShell(r *http.Request) error {
	value := r.FormValue("value")
	// ok: go.http-command-injection
	return exec.CommandContext(r.Context(), "printf", "%s", value).Run()
}
