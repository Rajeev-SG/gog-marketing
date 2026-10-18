package runner

import (
	"slices"
	"testing"
)

func TestNativeBindingMetadataCannotMutateRuntimePolicy(t *testing.T) {
	binding, ok := nativeBinding("appscript_get")
	if !ok || len(binding.scopes) == 0 || len(binding.methods) == 0 {
		t.Fatal("expected the existing active Apps Script read binding")
	}
	scopes := slices.Clone(binding.scopes)
	methods := slices.Clone(binding.methods)

	t.Cleanup(func() {
		copy(binding.scopes, scopes)
		copy(binding.methods, methods)
	})

	for _, metadata := range NativeBindingMetadata() {
		if metadata.Name != binding.Name {
			continue
		}
		metadata.Scopes[0] = "untrusted-inventory-scope"
		metadata.Methods[0] = "DELETE"

		actual, _ := nativeBinding(binding.Name)
		if !slices.Equal(actual.scopes, scopes) || !slices.Equal(actual.methods, methods) {
			t.Fatal("inventory caller changed live authorization policy")
		}

		return
	}

	t.Fatal("binding was absent from metadata")
}
