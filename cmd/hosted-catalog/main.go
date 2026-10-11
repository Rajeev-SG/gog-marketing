// hosted-catalog emits the executable native hosted schemas for the Worker.
// It performs no authenticated operations and never initializes CLI credentials.
package main

import (
	"encoding/json"
	"os"

	"github.com/openclaw/gogcli/internal/hosted/runner"
)

func main() {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(runner.NativeToolCatalog()); err != nil {
		os.Exit(1)
	}
}
