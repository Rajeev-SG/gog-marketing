package runner

import (
	"net/http"
	"strings"
)

func nativeDocsStructuralBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 18)

	for _, name := range []string{"docs_insert_footnote", "docs_insert_horizontal_rule", "docs_insert_page_break", "docs_insert_section_break", "docs_insert_table", "docs_insert_person", "docs_insert_date_chip", "docs_header_create", "docs_footer_create", "docs_header_delete", "docs_footer_delete", "docs_table_row_insert", "docs_table_row_delete", "docs_table_column_insert", "docs_table_column_delete", "docs_table_merge", "docs_table_unmerge", "docs_table_row_pin_header"} {
		binding := nativeDocsInventoryBinding(name)
		binding.Write = true
		binding.destructive = strings.HasSuffix(name, "_delete")
		binding.scopes = []string{"documents", driveFileScope}
		binding.methods = []string{http.MethodGet, http.MethodPost}
		selected := binding.argumentPaths
		binding.argumentPaths = func(args map[string]any) ([]string, error) {
			paths, err := selected(args)
			if err != nil {
				return nil, err
			}

			return []string{paths[0], paths[0] + ":batchUpdate"}, nil
		}
		bindings = append(bindings, binding)
	}

	return bindings
}
