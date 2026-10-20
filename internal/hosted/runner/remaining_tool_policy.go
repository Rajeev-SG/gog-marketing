package runner

import (
	"net/http"
	"slices"
)

// New metadata-derived same-file routes are admitted separately; every inherited
// tool retains its original exact-path decision.
func nativeRemainingToolPathAllowed(invocation *ToolInvocation, request *http.Request, paths []string, path string) bool {
	if handled, allowed := admitSheetsRemainingDynamicPath(invocation, request); handled {
		return allowed
	}

	if handled, allowed := admitDocsDriveRemainingRequest(invocation, request); handled {
		return allowed
	}

	return slices.Contains(paths, path)
}
