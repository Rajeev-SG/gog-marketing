package runner

import "net/http"

func nativeDocsFormatLayoutBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 3)

	for _, name := range []string{"docs_format", "docs_page_layout", "docs_section_columns"} {
		binding := nativeDocsInventoryBinding(name)
		binding.Write = true
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
