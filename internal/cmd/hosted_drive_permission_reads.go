package cmd

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedDrivePermissionReadArgument = errors.New("invalid Drive permission read argument")

func hostedDrivePermissionReadToken(raw any) (string, bool) {
	token, ok := raw.(string)
	if !ok || len(token) > 4096 || strings.IndexFunc(token, unicode.IsControl) >= 0 || strings.TrimSpace(token) != token {
		return "", false
	}
	return token, true
}

func hostedDrivePermissionReadStrictBool(raw any) (bool, bool) {
	value, ok := raw.(bool)
	return value, ok
}

func hostedDrivePermissionReadUser(raw any) (string, error) {
	email, ok := raw.(string)
	if !ok {
		return "", errHostedDrivePermissionReadArgument
	}
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 320 || strings.IndexFunc(email, unicode.IsControl) >= 0 {
		return "", errHostedDrivePermissionReadArgument
	}
	return email, nil
}

func hostedDrivePermissionReadDomains(raw any) ([]string, error) {
	var rawItems []any
	switch items := raw.(type) {
	case []any:
		rawItems = items
	case []string:
		rawItems = make([]any, 0, len(items))
		for _, item := range items {
			rawItems = append(rawItems, item)
		}
	default:
		return nil, errHostedDrivePermissionReadArgument
	}
	if len(rawItems) > 100 {
		return nil, errHostedDrivePermissionReadArgument
	}
	domains := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		domain, ok := item.(string)
		if !ok {
			return nil, errHostedDrivePermissionReadArgument
		}
		domain = strings.TrimSpace(domain)
		if domain == "" || len(domain) > 253 || strings.IndexFunc(domain, unicode.IsControl) >= 0 {
			return nil, errHostedDrivePermissionReadArgument
		}
		domains = append(domains, domain)
	}
	return domains, nil
}

func hostedDrivePermissionReadTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "drive_permissions", Service: "drive", Risk: mcpRiskRead,
			Description: "List one bounded page of permission metadata for an explicitly selected Drive file. Returns a next-page token when available; no content downloads, no automatic page traversal, and no access to other files. Returned hyperlinks are metadata only and are never fetched. Requires existing drive.file scope and exact tool/file grants; no write approval or new consent.",
			Options: []mcp.ToolOption{
				mcp.WithString("file_id", mcp.Required()),
				mcp.WithInteger("max_results", mcp.Min(1), mcp.Max(100)),
				mcp.WithString("page_token"),
			},
			BuildArgs: func(request mcp.CallToolRequest) ([]string, error) {
				id, err := hostedDriveRevisionID(request, "file_id")
				if err != nil {
					return nil, err
				}
				limit := 100
				if raw, present := request.GetArguments()["max_results"]; present {
					limit, err = validateHostedTabIndexArg(raw)
					if err != nil || limit < 1 || limit > 100 {
						return nil, errHostedDrivePermissionReadArgument
					}
				}
				args := []string{"drive", "permissions", "--max=" + strconv.Itoa(limit)}
				if raw, present := request.GetArguments()["page_token"]; present {
					token, valid := hostedDrivePermissionReadToken(raw)
					if !valid {
						return nil, errHostedDrivePermissionReadArgument
					}
					if token != "" {
						args = append(args, "--page="+token)
					}
				}
				return append(args, "--", id), nil
			},
		},
		{
			Name: "drive_audit_sharing", Service: "drive", Risk: mcpRiskRead,
			Description: "Run a native Drive sharing audit for one explicitly selected Drive file only, optionally filtered to public-only or external-only results and with optional internal-domain classification overrides. No parent traversal, children enumeration, content downloads, or access beyond the granted file. Returned hyperlinks are metadata only and are never fetched. Requires existing drive.file scope and exact tool/file grants; no write approval or new consent.",
			Options: []mcp.ToolOption{
				mcp.WithString("file_id", mcp.Required()),
				mcp.WithBoolean("public_only"),
				mcp.WithBoolean("external_only"),
				mcp.WithArray("internal_domains", mcp.WithStringItems(), mcp.MaxItems(100)),
			},
			BuildArgs: func(request mcp.CallToolRequest) ([]string, error) {
				id, err := hostedDriveRevisionID(request, "file_id")
				if err != nil {
					return nil, err
				}
				args := []string{"drive", "audit", "sharing", "--file=" + id}
				publicOnly := false
				externalOnly := false
				if raw, present := request.GetArguments()["public_only"]; present {
					value, ok := hostedDrivePermissionReadStrictBool(raw)
					if !ok {
						return nil, errHostedDrivePermissionReadArgument
					}
					publicOnly = value
					args = append(args, "--public-only="+strconv.FormatBool(value))
				}
				if raw, present := request.GetArguments()["external_only"]; present {
					value, ok := hostedDrivePermissionReadStrictBool(raw)
					if !ok {
						return nil, errHostedDrivePermissionReadArgument
					}
					externalOnly = value
					args = append(args, "--external-only="+strconv.FormatBool(value))
				}
				if publicOnly && externalOnly {
					return nil, errHostedDrivePermissionReadArgument
				}
				if raw, present := request.GetArguments()["internal_domains"]; present {
					domains, err := hostedDrivePermissionReadDomains(raw)
					if err != nil {
						return nil, err
					}
					for _, domain := range domains {
						args = append(args, "--internal-domain="+domain)
					}
				}
				return args, nil
			},
		},
		{
			Name: "drive_audit_user", Service: "drive", Risk: mcpRiskRead,
			Description: "Run a native Drive user sharing audit for one explicitly selected Drive file only, filtered locally to a single required user email. The email is a local filter, not impersonation. No parent traversal, children enumeration, content downloads, or access beyond the granted file. Returned hyperlinks are metadata only and are never fetched. Requires existing drive.file scope and exact tool/file grants; no write approval or new consent.",
			Options: []mcp.ToolOption{
				mcp.WithString("file_id", mcp.Required()),
				mcp.WithString("user", mcp.Required()),
			},
			BuildArgs: func(request mcp.CallToolRequest) ([]string, error) {
				id, err := hostedDriveRevisionID(request, "file_id")
				if err != nil {
					return nil, err
				}
				email, err := hostedDrivePermissionReadUser(request.GetArguments()["user"])
				if err != nil {
					return nil, err
				}
				return []string{"drive", "audit", "user", "--file=" + id, "--", email}, nil
			},
		},
	}
}
