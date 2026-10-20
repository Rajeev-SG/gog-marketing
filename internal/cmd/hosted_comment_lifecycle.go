package cmd

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Both services delegate comment mutations to the same native Drive helpers.
func hostedCommentLifecycleTools() []mcpToolSpec {
	specs := make([]mcpToolSpec, 0, 14)
	for _, service := range []string{"docs", "drive"} {
		verbs := []string{"create", "update", "delete", "reply", "resolve", "reopen"}
		if service == "docs" {
			verbs = []string{"add", "delete", "get", "list", "locate", "reply", "resolve", "reopen"}
		}
		for _, verb := range verbs {
			idKey := "file_id"
			if service == "docs" {
				idKey = "document_id"
			}
			options := []mcp.ToolOption{mcp.WithString(idKey, mcp.Required())}
			if verb != "create" && verb != "add" && verb != "list" {
				options = append(options, mcp.WithString("comment_id", mcp.Required()))
			}
			if verb == "create" || verb == "add" || verb == "update" || verb == "reply" {
				options = append(options, mcp.WithString("content", mcp.Required()))
			}
			if verb == "create" || verb == "add" {
				options = append(options, mcp.WithString("quoted"))
			}
			if verb == "resolve" || verb == "reopen" {
				options = append(options, mcp.WithString("message"))
			}
			if verb == "reply" {
				options = append(options, mcp.WithString("action", mcp.Enum("resolve", "reopen")))
			}
			if verb == "list" {
				options = append(options, mcp.WithString("page"), mcp.WithBoolean("include_resolved"))
			}
			risk := mcpRiskWrite
			if verb == "list" || verb == "get" || verb == "locate" {
				risk = mcpRiskRead
			}
			specs = append(specs, mcpToolSpec{Name: service + "_comments_" + verb, Service: service, Risk: risk, Description: "Native " + service + " comment " + verb + " on one explicitly granted file, using existing drive.file authority. Exact tool/file grants; writes require one-use chat approval and deletion additionally requires destructive authority. Inline bounded text only, no arbitrary anchors, local files, all-pages options or impersonation. Mutations are single-attempt.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedCommentLifecycleArgs(service, verb, req) }})
		}
	}
	return specs
}

func hostedCommentLifecycleArgs(service, verb string, req mcp.CallToolRequest) ([]string, error) {
	idKey := "file_id"
	if service == "docs" {
		idKey = "document_id"
	}
	id, err := hostedDocsNamedEditString(req, idKey, 256)
	if err != nil {
		return nil, err
	}
	if !hostedDocsResourceIDPattern.MatchString(id) {
		return nil, errHostedCommentsID
	}
	args := []string{service, "comments", verb}
	for _, key := range []string{"quoted", "message", "action", "page"} {
		if _, present := req.GetArguments()[key]; present {
			value, e := hostedDocsNamedEditString(req, key, 262144)
			if e != nil {
				return nil, e
			}
			if key == "action" && value != "resolve" && value != "reopen" {
				return nil, errHostedDocsNamedEditArgument
			}
			args = append(args, "--"+key+"="+value)
		}
	}
	if verb == "list" {
		args = append(args, "--max=50")
		include, e := hostedDocsNamedEditBool(req, "include_resolved", false)
		if e != nil {
			return nil, e
		}
		if include {
			args = append(args, "--include-resolved")
		}
	}
	args = append(args, "--", id)
	if verb != "create" && verb != "add" && verb != "list" {
		comment, e := hostedDocsNamedEditString(req, "comment_id", 256)
		if e != nil {
			return nil, e
		}
		if !hostedDocsResourceIDPattern.MatchString(comment) {
			return nil, errHostedCommentsID
		}
		args = append(args, comment)
	}
	if verb == "create" || verb == "add" || verb == "update" || verb == "reply" {
		value, e := hostedDocsNamedEditString(req, "content", 262144)
		if e != nil {
			return nil, e
		}
		if strings.TrimSpace(value) == "" {
			return nil, errHostedDocsNamedEditArgument
		}
		args = append(args, value)
	}
	return args, nil
}
