package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AppScriptPushCmd struct {
	ScriptID  string `arg:"" name:"scriptId" help:"Script ID"`
	Dir       string `name:"dir" help:"Local flat project directory; alternative to --files-json" type:"path"`
	FilesJSON string `name:"files-json" help:"Literal JSON array of name, type and source; replaces ALL project files"`
}
type AppScriptVersionCreateCmd struct {
	ScriptID    string `arg:"" name:"scriptId" help:"Script ID"`
	Description string `name:"description" help:"Version description"`
}
type AppScriptDeploymentCreateCmd struct {
	ScriptID         string `arg:"" name:"scriptId" help:"Script ID"`
	Version          int64  `name:"version-number" required:"" help:"Immutable version number (not HEAD)"`
	Description      string `name:"description" help:"Deployment description"`
	ManifestFileName string `name:"manifest-file-name" help:"Optional manifest filename; omitted to use Google's default"`
}

func (c *AppScriptPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if id == "" {
		return usage("script id is required")
	}

	files, err := readAppScriptPushFiles(c.Dir, c.FilesJSON)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"script_id":  id,
		"file_count": len(files),
	}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "appscript.push", payload, "replace Apps Script project files"); confirmErr != nil {
		return confirmErr
	}

	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	reply, err := svc.Projects.UpdateContent(id, &scriptapi.Content{Files: files}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("push Apps Script project: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"pushed":  true,
			"content": reply,
		})
	}

	ui.FromContext(ctx).Out().Linef("script_id: %s", id)
	ui.FromContext(ctx).Out().Linef("files: %d", len(files))
	return nil
}

func (c *AppScriptVersionCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if id == "" {
		return usage("empty scriptId")
	}

	if err := dryRunExit(ctx, flags, "appscript.version.create", map[string]any{
		"script_id":   id,
		"description": c.Description,
	}); err != nil {
		return err
	}

	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	reply, err := svc.Projects.Versions.Create(id, &scriptapi.Version{
		Description: c.Description,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("creating Apps Script version: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"created": true,
			"version": reply,
		})
	}

	ui.FromContext(ctx).Out().Linef("script_id: %s", id)
	ui.FromContext(ctx).Out().Linef("version_number: %d", reply.VersionNumber)
	return nil
}

func (c *AppScriptDeploymentCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if id == "" {
		return usage("script id is required")
	}
	if c.Version < 1 || c.Version > 2147483647 {
		return usage("version must be a positive integer between 1 and 2147483647")
	}
	if err := dryRunExit(ctx, flags, "appscript.deployment.create", map[string]any{
		"script_id":          id,
		"version_number":     c.Version,
		"description":        c.Description,
		"manifest_file_name": c.ManifestFileName,
	}); err != nil {
		return err
	}
	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}
	reply, err := svc.Projects.Deployments.Create(id, &scriptapi.DeploymentConfig{
		VersionNumber:    c.Version,
		Description:      c.Description,
		ManifestFileName: c.ManifestFileName,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"created":    true,
			"deployment": reply,
		})
	}
	ui.FromContext(ctx).Out().Linef("script_id: %s", id)
	ui.FromContext(ctx).Out().Linef("deployment_id: %s", reply.DeploymentId)
	return nil
}

func decodeAppScriptPushFiles(raw string) ([]*scriptapi.File, error) {
	if len(raw) > 8<<20 {
		return nil, usage("input exceeds maximum size of 8MiB")
	}
	if !strings.HasPrefix(strings.TrimSpace(raw), "[") {
		return nil, usage("input must be a literal JSON array")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var files []struct {
		Name   string  `json:"name"`
		Type   string  `json:"type"`
		Source *string `json:"source"`
	}
	if err := dec.Decode(&files); err != nil {
		return nil, usage("invalid JSON input: " + err.Error())
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, usage("unexpected trailing data after JSON array")
	}
	if len(files) == 0 {
		return nil, usage("at least one file is required")
	}
	if len(files) > 1000 {
		return nil, usage("more than 1000 files is not allowed")
	}
	seen := make(map[string]bool, len(files))
	manifest := 0
	totalSource := 0
	out := make([]*scriptapi.File, 0, len(files))
	for i := range files {
		f := &files[i]
		if f.Source == nil {
			return nil, usage(fmt.Sprintf("file %q: missing source", f.Name))
		}
		if strings.TrimSpace(f.Name) == "" {
			return nil, usage("file name must not be blank")
		}
		if len(f.Name) > 1024 {
			return nil, usage(fmt.Sprintf("file name %q exceeds 1024 bytes", f.Name))
		}
		if strings.IndexFunc(f.Name, unicode.IsControl) != -1 {
			return nil, usage(fmt.Sprintf("file name %q contains control characters", f.Name))
		}
		if f.Type != "SERVER_JS" && f.Type != "HTML" && f.Type != "JSON" {
			return nil, usage(fmt.Sprintf("file %q: invalid type %q", f.Name, f.Type))
		}
		key := f.Type + "\x00" + f.Name
		if seen[key] {
			return nil, usage(fmt.Sprintf("duplicate file %q with type %q", f.Name, f.Type))
		}
		seen[key] = true
		if f.Type == "JSON" {
			if f.Name != "appsscript" {
				return nil, usage(fmt.Sprintf("JSON file %q must be named appsscript", f.Name))
			}
			manifest++
			if !strings.HasPrefix(strings.TrimSpace(*f.Source), "{") || !json.Valid([]byte(*f.Source)) {
				return nil, usage("appsscript manifest source must be a valid JSON object")
			}
		}
		totalSource += len(*f.Source)
		if totalSource > 5<<20 {
			return nil, usage("total source size exceeds 5MiB limit")
		}
		out = append(out, &scriptapi.File{Name: f.Name, Type: f.Type, Source: *f.Source})
	}
	if manifest != 1 {
		return nil, usage("exactly one appsscript JSON manifest file is required")
	}
	return out, nil
}

type AppScriptBoundCreateCmd struct {
	Title    string `name:"title" required:"" help:"Project title"`
	ParentID string `name:"parent-id" required:"" help:"Explicit parent document, sheet, slide or form ID"`
}

func (c *AppScriptBoundCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if strings.TrimSpace(normalizeGoogleID(c.ParentID)) == "" {
		return usage("empty --parent-id")
	}
	return (&AppScriptCreateCmd{Title: c.Title, ParentID: c.ParentID}).Run(ctx, flags)
}
