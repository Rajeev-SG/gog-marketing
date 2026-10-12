package runner

import (
	"net/http"
	"net/mail"
	"regexp"

	"github.com/openclaw/gogcli/internal/hosted/native"
)

var groupMembershipPath = regexp.MustCompile(`^/v1/groups/[A-Za-z0-9_-]{1,256}/memberships$`)

func validateGroupEmail(args map[string]any) ([]string, error) {
	group, _ := args["group_email"].(string)

	address, err := mail.ParseAddress(group)
	if err != nil || address.Address != group {
		return nil, invalidf("invalid group email")
	}

	return []string{}, nil
}

func admitGroupMembershipRequest(invocation *ToolInvocation, request *http.Request) error {
	if request.Method != http.MethodGet {
		return native.ErrDenied
	}

	if request.URL.Path == "/v1/groups:lookup" {
		group, _ := invocation.Arguments["group_email"].(string)
		if request.URL.Query().Get("groupKey.id") == group {
			return nil
		}
	}

	if groupMembershipPath.MatchString(request.URL.Path) {
		return nil
	}

	return native.ErrDenied
}
