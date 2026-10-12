package runner

import (
	"net/http"

	"github.com/openclaw/gogcli/internal/hosted/policy"
)

func nativeTagManagerBindings() []NativeToolBinding {
	operations := policy.TagManagerOperations()
	result := make([]NativeToolBinding, 0, len(operations))

	for _, op := range operations {
		scopes := []string{op.Scope}
		if op.Method == http.MethodGet {
			scopes = []string{"tagmanager.readonly"}
			if op.Family != "accounts" {
				scopes = append(scopes, "tagmanager.edit.containers")
			}

			if op.Family == "versions" {
				scopes = append(scopes, "tagmanager.edit.containerversions")
			}
		}
		result = append(result, NativeToolBinding{Name: op.Name(), Service: "tagmanager", ResourceType: op.Selector, ResourceArgument: op.ResourceArgument(), Write: op.Method != http.MethodGet, hosts: []string{"tagmanager.googleapis.com"}, scopes: scopes, methods: []string{op.Method}, destructive: op.Destructive, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			path, err := op.Path(args)
			if err != nil {
				return nil, invalidf("invalid Tag Manager target")
			}

			return []string{path}, nil
		}})
	}

	return result
}
