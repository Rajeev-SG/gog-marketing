package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedContactsTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 6)
	for _, operation := range []string{"list", "search", "get", "create", "update", "delete"} {
		risk := mcpRiskRead
		options := []mcp.ToolOption{}
		stringsFields := []string{}
		arrays := []string{}
		if operation == "create" || operation == "update" || operation == "delete" {
			risk = mcpRiskWrite
		}
		if operation == "get" || operation == "update" || operation == "delete" {
			options = append(options, mcp.WithString("resource_name", mcp.Required(), mcp.Description("Explicit people/... resource name, not email lookup")))
		}
		if operation == "search" {
			options = append(options, mcp.WithString("query", mcp.Required()))
		}
		if operation == "list" || operation == "search" {
			maximum := float64(1000)
			if operation == "search" {
				maximum = 30
			}
			options = append(options, mcp.WithInteger("max", mcp.Min(1), mcp.Max(maximum)))
		}
		if operation == "list" {
			stringsFields = append(stringsFields, "page")
		}
		if operation == "create" || operation == "update" {
			stringsFields = append(stringsFields, "given", "family", "email", "phone", "org", "title", "note", "gender")
			arrays = []string{"url", "address", "custom", "relation"}
			if operation == "update" {
				stringsFields = append(stringsFields, "birthday", "notes")
			}
		}
		for _, key := range stringsFields {
			if operation == "create" && key == "given" {
				options = append(options, mcp.WithString(key, mcp.Required()))
			} else {
				options = append(options, mcp.WithString(key))
			}
		}
		for _, key := range arrays {
			options = append(options, mcp.WithArray(key, mcp.WithStringItems(), mcp.MaxItems(100)))
		}
		result = append(result, mcpToolSpec{
			Name: "contacts_" + operation, Service: "contacts", Risk: risk, Description: "Run native Contacts " + operation + " in an explicitly enabled account. Update retains native etag protection. Deletion requires destructive permission; file input/etag bypass is not exposed.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				args := []string{"contacts", operation}
				fields := req.GetArguments()
				for _, key := range stringsFields {
					if _, present := fields[key]; present {
						args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
					}
				}
				for _, key := range arrays {
					if raw, present := fields[key]; present {
						values, ok := raw.([]any)
						if !ok {
							return nil, ErrHostedToolInput
						}
						// Native empty singleton explicitly clears an array field.
						if len(values) == 0 {
							args = append(args, "--"+key+"=")
						}
						for _, value := range values {
							text, ok := value.(string)
							if !ok {
								return nil, ErrHostedToolInput
							}
							args = append(args, "--"+key+"="+text)
						}
					}
				}
				if operation == "list" || operation == "search" {
					defaultMax := 100
					if operation == "search" {
						defaultMax = 30
					}
					args = append(args, "--max="+strconv.Itoa(req.GetInt("max", defaultMax)))
				}
				args = append(args, "--")
				if operation == "search" {
					query, err := requireMCPString(req, "query")
					if err != nil {
						return nil, err
					}
					args = append(args, query)
				}
				if operation == "get" || operation == "update" || operation == "delete" {
					id, err := requireMCPString(req, "resource_name")
					if err != nil {
						return nil, err
					}
					args = append(args, id)
				}
				return args, nil
			},
		})
	}
	return result
}
