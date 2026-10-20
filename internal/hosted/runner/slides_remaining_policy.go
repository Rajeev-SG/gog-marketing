package runner

import (
	"net/http"
	"strings"

	"github.com/openclaw/gogcli/internal/hosted/native"
)

func nativeSlidesRemainingBindings() []NativeToolBinding {
	names := []string{"slides_bullets", "slides_copy", "slides_element_alt_text", "slides_element_create_line", "slides_element_create_shape", "slides_element_delete", "slides_element_group", "slides_element_style", "slides_element_transform", "slides_element_ungroup", "slides_element_z_order", "slides_insert_text", "slides_link", "slides_paragraph_style", "slides_raw", "slides_replace_slide", "slides_style_text", "slides_thumbnail"}

	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		write := name != "slides_raw" && name != "slides_thumbnail"

		binding := NativeToolBinding{Name: name, Service: slidesLifecycleService, ResourceType: resourceTypeFile, ResourceArgument: slidesLifecyclePresentationArg, Write: write, destructive: name == "slides_element_delete" || name == "slides_replace_slide", hosts: []string{nativeSlidesHost}, scopes: []string{"presentations", driveFileScope}, methods: []string{http.MethodGet}, paths: exactPaths("", "")}
		if write {
			binding.methods = []string{http.MethodGet, http.MethodPost}
		} else {
			binding.scopes = append(binding.scopes, "presentations.readonly")
		}

		if name == "slides_copy" {
			binding.hosts = []string{nativeDocsSlidesDriveHost}
			binding.scopes = []string{driveFileScope}
		}
		binding.argumentPaths = func(args map[string]any) ([]string, error) {
			id, _ := args[slidesLifecyclePresentationArg].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid presentation")
			}

			base := "/v1/presentations/" + id
			if name == "slides_copy" {
				base = "/drive/v3/files/" + id
				return []string{base, base + "/copy"}, nil
			}

			if name == "slides_thumbnail" {
				page, _ := args["slide_id"].(string)
				if !nativeSlidesObjectSelectorValid(page) {
					return nil, invalidf("invalid page")
				}

				return []string{base + "/pages/" + page + "/thumbnail"}, nil
			}

			if write {
				return []string{base, base + ":batchUpdate"}, nil
			}

			return []string{base}, nil
		}
		bindings = append(bindings, binding)
	}

	return bindings
}

func admitSlidesRemainingMethod(name, path, method string) error {
	found := false

	for _, binding := range nativeSlidesRemainingBindings() {
		if binding.Name == name {
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	mutation := strings.HasSuffix(path, ":batchUpdate") || strings.HasSuffix(path, "/copy")
	if (mutation && method == http.MethodPost) || (!mutation && method == http.MethodGet) {
		return nil
	}

	return native.ErrDenied
}

func validateSlidesRemainingClear(invocation *ToolInvocation) error {
	if invocation.Name == "slides_insert_text" {
		replace, _ := invocation.Arguments["replace"].(bool)

		text, _ := invocation.Arguments["text"].(string)
		if replace && text == "" && !invocation.Authorization.AllowDestructive {
			return native.ErrDenied
		}
	}

	return nil
}
