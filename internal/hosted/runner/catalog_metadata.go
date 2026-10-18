package runner

// BindingMetadata is a read-only copy of static runner admission metadata.
// It does not alter validation or add fields to the executable catalogue.
type BindingMetadata struct {
	Name        string   `json:"name"`
	Service     string   `json:"service"`
	Scopes      []string `json:"scopes"`
	Methods     []string `json:"methods"`
	Destructive bool     `json:"destructive"`
}

// NativeBindingMetadata copies existing binding metadata for inventory tools.
// An empty Methods slice means the binding intentionally has no single static
// method claim; callers must expose that as unknown rather than infer methods.
func NativeBindingMetadata() []BindingMetadata {
	out := make([]BindingMetadata, 0, len(nativeToolBindings))
	for _, binding := range nativeToolBindings {
		metadata := BindingMetadata{
			Name:        binding.Name,
			Service:     binding.Service,
			Destructive: binding.destructive,
		}

		if len(binding.scopes) > 0 {
			metadata.Scopes = append([]string(nil), binding.scopes...)
		}

		if len(binding.methods) > 0 {
			metadata.Methods = append([]string(nil), binding.methods...)
		}

		out = append(out, metadata)
	}

	return out
}
