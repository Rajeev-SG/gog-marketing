package runner

import "net/http"

func nativeDocsTextEditBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 4)

	for _, name := range []string{"docs_insert", "docs_delete", "docs_clear", "docs_find_replace"} {
		binding := nativeDocsInventoryBinding(name)
		binding.Write = true
		binding.destructive = name == "docs_delete" || name == "docs_clear"
		binding.scopes = []string{"documents", driveFileScope}
		binding.methods = []string{http.MethodGet, http.MethodPost}
		selected := binding.argumentPaths
		binding.argumentPaths = func(args map[string]any) ([]string, error) {
			paths, err := selected(args)
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
