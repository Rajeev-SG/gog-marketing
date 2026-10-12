package policy

import (
	"errors"
	"net/http"
	"regexp"
)

// TagManagerOperation is a fixed existing native command, never arbitrary argv.
type TagManagerOperation struct {
	Family, Action, Selector, Method, Scope string
	Workspace, Item, Destructive            bool
}

var (
	errTagManagerSelector   = errors.New("invalid Tag Manager selector")
	tagManagerAccountPath   = regexp.MustCompile(`^accounts/[0-9]{1,128}$`)
	tagManagerContainerPath = regexp.MustCompile(`^accounts/[0-9]{1,128}/containers/[0-9]{1,128}$`)
	tagManagerChildID       = regexp.MustCompile(`^[0-9]{1,128}$`)
)

func TagManagerOperations() []TagManagerOperation {
	result := make([]TagManagerOperation, 0, 33)

	for _, family := range []string{"accounts", "containers", "workspaces", "tags", "triggers", "variables", "versions"} {
		actions := []string{"list", "get", "create", "update", "delete"}
		if family == "accounts" {
			actions = []string{"list", "get"}
		}

		if family == "versions" {
			actions = []string{"list", "get", "delete"}
		}

		if family == "workspaces" {
			actions = append(actions, "status", "sync", "create-version")
		}

		for _, action := range actions {
			op := TagManagerOperation{Family: family, Action: action, Selector: "container", Method: http.MethodGet, Scope: "tagmanager.edit.containers", Destructive: action == "delete"}
			switch action {
			case "create", "sync", "create-version":
				op.Method = http.MethodPost
			case "update":
				op.Method = http.MethodPut
			case "delete":
				op.Method = http.MethodDelete
			}

			if family == "accounts" {
				op.Selector = "account"
				if action == "list" {
					op.Selector = ""
				}
			}

			if family == "containers" && (action == "list" || action == "create") {
				op.Selector = "account"
			}
			op.Workspace = family == "tags" || family == "triggers" || family == "variables" || (family == "workspaces" && action != "list" && action != "create")

			op.Item = (family == "tags" || family == "triggers" || family == "variables" || family == "versions") && action != "list" && action != "create"
			if action == "delete" && (family == "containers" || family == "workspaces") {
				op.Scope = "tagmanager.delete.containers"
			}

			if (family == "versions" && action == "delete") || action == "create-version" {
				op.Scope = "tagmanager.edit.containerversions"
			}

			result = append(result, op)
		}
	}

	return result
}

func (op TagManagerOperation) Name() string { return "tagmanager_" + op.Family + "_" + op.Action }
func (op TagManagerOperation) ResourceArgument() string {
	if op.Selector == "" {
		return ""
	}

	return op.Selector + "_path"
}

func (op TagManagerOperation) Path(args map[string]any) (string, error) {
	path, _ := args[op.ResourceArgument()].(string)
	switch op.Selector {
	case "":
		path = "accounts"
	case "account":
		if !tagManagerAccountPath.MatchString(path) {
			return "", errTagManagerSelector
		}
	case "container":
		if !tagManagerContainerPath.MatchString(path) {
			return "", errTagManagerSelector
		}
	default:
		return "", errTagManagerSelector
	}

	if op.Family == "accounts" {
		return "/tagmanager/v2/" + path, nil
	}

	if op.Family == "containers" {
		if op.Action == "list" || op.Action == "create" {
			path += "/containers"
		}

		return "/tagmanager/v2/" + path, nil
	}

	if op.Family == "versions" {
		if op.Action == "list" {
			path += "/version_headers"
		} else {
			id, _ := args["version_id"].(string)
			if !tagManagerChildID.MatchString(id) {
				return "", errTagManagerSelector
			}
			path += "/versions/" + id
		}

		return "/tagmanager/v2/" + path, nil
	}
	path += "/workspaces"

	if op.Workspace {
		id, _ := args["workspace_id"].(string)
		if !tagManagerChildID.MatchString(id) {
			return "", errTagManagerSelector
		}
		path += "/" + id
	}

	if op.Family != "workspaces" {
		path += "/" + op.Family
		if op.Item {
			id, _ := args["resource_id"].(string)
			if !tagManagerChildID.MatchString(id) {
				return "", errTagManagerSelector
			}
			path += "/" + id
		}
	}

	switch op.Action {
	case "status":
		path += "/status"
	case "sync":
		path += ":sync"
	case "create-version":
		path += ":create_version"
	}

	return "/tagmanager/v2/" + path, nil
}
