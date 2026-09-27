package secrets

import (
	"os"
	"runtime"
	"testing"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/termutil"
)

func testSystemLayout(tb testing.TB, kinds ...config.PathKind) config.Layout {
	tb.Helper()

	layout, err := config.NewSystemResolver("").Resolve(kinds...)
	if err != nil {
		tb.Fatalf("resolve test layout: %v", err)
	}

	return layout
}

func openSystemTestStore(tb testing.TB) Repository {
	tb.Helper()

	layout := testSystemLayout(tb, config.PathKindConfig, config.PathKindData)

	store, err := Open(systemTestOpenOptions(tb, layout, config.NewConfigStore(layout)))
	if err != nil {
		tb.Fatalf("open test store: %v", err)
	}

	return store
}

func systemTestOpenOptions(tb testing.TB, layout config.Layout, store *config.ConfigStore) OpenOptions {
	tb.Helper()

	options, err := OpenOptionsFromLookup(layout, store, os.LookupEnv, runtime.GOOS, termutil.IsTerminal(os.Stdin))
	if err != nil {
		tb.Fatal(err)
	}

	return options
}
