// Command hosted-preflight runs the read-only hosted v1 provider preflight.
// It emits JSON and never echoes provider subprocess output or secret values.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/openclaw/gogcli/internal/hosted/provider"
)

var (
	errLoadConfig   = errors.New("hosted-preflight: could not load provider inventory")
	errRunPreflight = errors.New("hosted-preflight: provider preflight failed")
	errEncodeReport = errors.New("hosted-preflight: could not encode report")
)

func main() {
	os.Exit(run())
}

func run() int {
	reportOnly := flag.Bool("report-only", false, "always exit 0 after printing the report")
	flag.Parse()

	config, err := provider.Load()
	if err != nil {
		return fail(errLoadConfig, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	report, err := provider.Preflight(ctx, config, provider.Options{Runner: provider.ExecRunner{}})
	if err != nil {
		return fail(errRunPreflight, err)
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fail(errEncodeReport, err)
	}
	fmt.Println(string(encoded))

	if !report.Ready && !*reportOnly {
		return 1
	}
	return 0
}

func fail(reason, cause error) int {
	fmt.Fprintf(os.Stderr, "%v: %v\n", reason, cause)
	return 2
}
