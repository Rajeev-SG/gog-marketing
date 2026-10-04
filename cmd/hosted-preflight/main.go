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
	errLoadConfig     = errors.New("hosted-preflight: could not load provider inventory")
	errRunPreflight   = errors.New("hosted-preflight: provider preflight failed")
	errEncodeReport   = errors.New("hosted-preflight: could not encode report")
	errInvalidTimeout = errors.New("hosted-preflight: invalid timeout")
	errTimeoutRange   = errors.New("--timeout and --command-timeout must be positive durations")
)

// Default deadlines are deliberately conservative: provider CLIs commonly
// exceed 30 seconds on cold environments (first credential/token refresh,
// first-run wrangler prompts, slow networks). See docs/hosted/provider.md.
const (
	defaultTotalTimeout   = 5 * time.Minute
	defaultCommandTimeout = 90 * time.Second
)

func main() {
	os.Exit(run())
}

func run() int {
	reportOnly := flag.Bool("report-only", false, "always exit 0 after printing the report")
	totalTimeout := flag.Duration("timeout", defaultTotalTimeout, "total preflight deadline (for example 30s, 5m; cold environments may need more)")
	commandTimeout := flag.Duration("command-timeout", defaultCommandTimeout, "deadline for each provider CLI command (for example 90s, 5m; cold environments may need several minutes)")
	flag.Parse()

	if *totalTimeout <= 0 || *commandTimeout <= 0 {
		return fail(errInvalidTimeout, errTimeoutRange)
	}

	config, err := provider.Load()
	if err != nil {
		return fail(errLoadConfig, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *totalTimeout)
	defer cancel()

	report, err := provider.Preflight(ctx, config, provider.Options{Runner: provider.ExecRunner{}, CommandTimeout: *commandTimeout})
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
