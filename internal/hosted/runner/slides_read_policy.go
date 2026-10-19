package runner

import (
	"net/http"
)

const (
	nativeSlidesDriveHost = "www.googleapis.com"
	nativeSlidesHost      = "slides.googleapis.com"
)

// nativeSlidesReadBindings registers the three fixed Slides reads. slides_info
// is the only read that must call Drive first; presentations scopes do not
// authorize that Drive GET, so only drive.file is admitted for it. Its paired
// paths cover the actual Drive and Slides calls; other reads are single-path.
func nativeSlidesReadBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{
			Name: "slides_info", Service: "slides",
			ResourceType: resourceTypeFile, ResourceArgument: "presentation_id",
			hosts: []string{nativeSlidesDriveHost, nativeSlidesHost}, methods: []string{http.MethodGet},
			scopes: []string{driveFileScope}, paths: slidesInfoPaths,
		},
		{
			Name: "slides_read_slide", Service: "slides",
			ResourceType: resourceTypeFile, ResourceArgument: "presentation_id",
			hosts: []string{nativeSlidesHost}, methods: []string{http.MethodGet},
			scopes: []string{"presentations.readonly", "presentations", driveFileScope},
			paths:  exactPaths("/v1/presentations/", ""), argumentPaths: nativeSlidesReadPaths,
		},
		{
			Name: "slides_locate", Service: "slides",
			ResourceType: resourceTypeFile, ResourceArgument: "presentation_id",
			hosts: []string{nativeSlidesHost}, methods: []string{http.MethodGet},
			scopes: []string{"presentations.readonly", "presentations", driveFileScope},
			paths:  exactPaths("/v1/presentations/", ""), argumentPaths: nativeSlidesReadPaths,
		},
	}
}

// validateSlidesReadArguments enforces fixed optional variants in policy and
// rejects unsupported booleans/counts before native execution.
func validateSlidesReadArguments(args map[string]any) error {
	for _, key := range []string{"detail", "match_case", "fail_empty", "all"} {
		if value, present := args[key]; present {
			if _, ok := value.(bool); !ok {
				return invalidf("invalid Slides read option")
			}
		}
	}

	if value, present := args["occurrence"]; present {
		valid := false

		switch n := value.(type) {
		case int:
			valid = n >= 1 && n <= 2147483647
		case int64:
			valid = n >= 1 && n <= 2147483647
		case float64:
			valid = n >= 1 && n <= 2147483647 && n == float64(int(n))
		}

		if !valid || args["all"] == true {
			return invalidf("invalid occurrence or conflicting selectors")
		}
	}

	return nil
}

// nativeSlidesObjectSelectorValid accepts short and colon-namespaced Slides
// object IDs (p and p:0) without turning the local selector into an extra
// resource grant.
func nativeSlidesObjectSelectorValid(value string) bool {
	if len(value) < 1 || len(value) > 256 {
		return false
	}
	first := func(char rune) bool {
		return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_'
	}

	rest := func(char rune) bool {
		return first(char) || char == ':' || char == '-'
	}
	for index, char := range value {
		if index == 0 && !first(char) || index > 0 && !rest(char) {
			return false
		}
	}

	return true
}

func slidesInfoPaths(id string) []string {
	return []string{"/drive/v3/files/" + id, "/v1/presentations/" + id}
}

func nativeSlidesReadPaths(args map[string]any) ([]string, error) {
	id, _ := args["presentation_id"].(string)
	if !fileResourceID.MatchString(id) {
		return nil, invalidf("invalid presentation selector")
	}

	if err := validateSlidesReadArguments(args); err != nil {
		return nil, err
	}

	for _, key := range []string{"slide_id", "page_id"} {
		if raw, present := args[key]; present {
			value, ok := raw.(string)
			if !ok || (value != "" && !nativeSlidesObjectSelectorValid(value)) {
				return nil, invalidf("invalid slide selector")
			}
		}
	}

	return []string{"/v1/presentations/" + id}, nil
}
