package runner

import "net/http"

func nativeDocsNamedEditBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 3)

	for _, name := range []string{"docs_named_range_create", "docs_named_range_delete", "docs_named_range_replace"} {
		binding := nativeDocsInventoryBinding(name)
		binding.Write = true
		binding.destructive = name == "docs_named_range_delete"
		binding.scopes = []string{"documents", driveFileScope}
		binding.methods = []string{http.MethodGet, http.MethodPost}
		selectedPaths := binding.argumentPaths
		binding.argumentPaths = func(args map[string]any) ([]string, error) {
			paths, err := selectedPaths(args)
			if err != nil {
				return nil, err
			}

			if len(paths) != 1 {
				return nil, invalidf("invalid selected document path")
			}

			return []string{paths[0], paths[0] + ":batchUpdate"}, nil
		}
		bindings = append(bindings, binding)
	}

	return bindings
}

func validateNativeDocsRangeClear(invocation *ToolInvocation) error {
	if ((invocation.Name == "docs_named_range_replace" && invocation.Arguments["text"] == "") || (invocation.Name == "docs_find_replace" && invocation.Arguments["replacement"] == "")) && !invocation.Authorization.AllowDestructive {
		return invalidf("clearing document text requires destructive permission")
	}

	return nil
}
