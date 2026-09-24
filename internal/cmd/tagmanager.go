package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/tagmanager/v2"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type TagManagerCmd struct {
	Accounts   TagManagerAccountsCmd   `cmd:"" help:"GTM accounts"`
	Containers TagManagerContainersCmd `cmd:"" help:"GTM containers"`
	Workspaces TagManagerWorkspacesCmd `cmd:"" help:"GTM workspaces"`
	Tags       TagManagerTagsCmd       `cmd:"" help:"GTM tags"`
	Triggers   TagManagerTriggersCmd   `cmd:"" help:"GTM triggers"`
	Variables  TagManagerVariablesCmd  `cmd:"" help:"GTM variables"`
	Versions   TagManagerVersionsCmd   `cmd:"" help:"GTM container versions"`
}

type TagManagerAccountsCmd struct {
	List TagManagerAccountsListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List GTM accounts"`
	Get  TagManagerAccountsGetCmd  `cmd:"" name:"get" aliases:"info,show" help:"Get a GTM account"`
}

type TagManagerAccountsListCmd struct {
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *TagManagerAccountsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	var items []*tagmanager.Account
	next := c.PageToken
	for {
		call := svc.Accounts.List().Context(ctx)
		if next != "" {
			call = call.PageToken(next)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return callErr
		}
		items = append(items, resp.Account...)
		next = resp.NextPageToken
		if !c.All || next == "" {
			break
		}
	}
	return writeTagManagerList(ctx, "accounts", items, next, c.FailEmpty, tagManagerAccountRow)
}

type TagManagerAccountsGetCmd struct {
	Account string `arg:"" name:"account" help:"GTM account ID or accounts/ID path"`
}

func (c *TagManagerAccountsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Get(tagManagerAccountPath(c.Account)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "account", item)
}

type TagManagerContainersCmd struct {
	List   TagManagerContainersListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List containers"`
	Get    TagManagerContainersGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get container"`
	Create TagManagerContainersCreateCmd `cmd:"" name:"create" help:"Create container"`
	Update TagManagerContainersUpdateCmd `cmd:"" name:"update" help:"Update container"`
	Delete TagManagerContainersDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove" help:"Delete container"`
}

type TagManagerContainersListCmd struct {
	Account   string `arg:"" name:"account"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *TagManagerContainersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	var items []*tagmanager.Container
	next := c.PageToken
	for {
		call := svc.Accounts.Containers.List(tagManagerAccountPath(c.Account)).Context(ctx)
		if next != "" {
			call = call.PageToken(next)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return callErr
		}
		items = append(items, resp.Container...)
		next = resp.NextPageToken
		if !c.All || next == "" {
			break
		}
	}
	return writeTagManagerList(ctx, "containers", items, next, c.FailEmpty, tagManagerContainerRow)
}

type TagManagerContainersGetCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
}

func (c *TagManagerContainersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Get(tagManagerContainerPath(c.Account, c.Container)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "container", item)
}

type TagManagerContainerMutation struct {
	Name         string   `name:"name"`
	Notes        string   `name:"notes"`
	UsageContext []string `name:"usage-context" sep:","`
	JSONFile     string   `name:"json-file" aliases:"body" help:"Complete Container JSON, inline or @file"`
}

type TagManagerContainersCreateCmd struct {
	Account                     string `arg:"" name:"account"`
	TagManagerContainerMutation `embed:""`
}

func (c *TagManagerContainersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	container, err := c.body()
	if err != nil {
		return err
	}
	if dryRunErr := marketingDryRunExit(ctx, flags, "tagmanager.containers.create", map[string]any{"account": c.Account, "container": container}); dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Create(tagManagerAccountPath(c.Account), container).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "container", item)
}

func (c *TagManagerContainersCreateCmd) body() (*tagmanager.Container, error) {
	out := &tagmanager.Container{Name: strings.TrimSpace(c.Name), Notes: c.Notes, UsageContext: c.UsageContext}
	if strings.TrimSpace(c.JSONFile) != "" {
		if err := decodeTagManagerJSON(c.JSONFile, out); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(out.Name) == "" {
		return nil, usage("container name is required")
	}
	return out, nil
}

type TagManagerContainersUpdateCmd struct {
	Account                     string `arg:"" name:"account"`
	Container                   string `arg:"" name:"container"`
	TagManagerContainerMutation `embed:""`
}

func (c *TagManagerContainersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	container, err := c.body()
	if err != nil {
		return err
	}
	path := tagManagerContainerPath(c.Account, c.Container)
	if dryRunErr := marketingDryRunExit(ctx, flags, "tagmanager.containers.update", map[string]any{"path": path, "container": container}); dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Update(path, container).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "container", item)
}

func (c *TagManagerContainersUpdateCmd) body() (*tagmanager.Container, error) {
	out := &tagmanager.Container{Name: strings.TrimSpace(c.Name), Notes: c.Notes, UsageContext: c.UsageContext}
	if strings.TrimSpace(c.JSONFile) != "" {
		if err := decodeTagManagerJSON(c.JSONFile, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type TagManagerContainersDeleteCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
}

func (c *TagManagerContainersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerContainerPath(c.Account, c.Container)
	if err := marketingDryRunAndConfirmDestructive(ctx, flags, "tagmanager.containers.delete", map[string]any{"path": path}, "delete GTM container "+path); err != nil {
		return err
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	if err := svc.Accounts.Containers.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("deleted", true), kv("path", path))
}

type TagManagerWorkspacesCmd struct {
	List          TagManagerWorkspacesListCmd         `cmd:"" default:"withargs" aliases:"ls"`
	Get           TagManagerWorkspacesGetCmd          `cmd:"" name:"get" aliases:"info,show"`
	Create        TagManagerWorkspacesCreateCmd       `cmd:"" name:"create"`
	Update        TagManagerWorkspacesUpdateCmd       `cmd:"" name:"update"`
	Delete        TagManagerWorkspacesDeleteCmd       `cmd:"" name:"delete" aliases:"rm,remove"`
	Status        TagManagerWorkspacesStatusCmd       `cmd:"" name:"status"`
	Sync          TagManagerWorkspacesSyncCmd         `cmd:"" name:"sync"`
	CreateVersion TagManagerWorkspaceCreateVersionCmd `cmd:"" name:"create-version"`
}

type TagManagerWorkspaceMutation struct {
	Name        string `name:"name"`
	Description string `name:"description"`
	JSONFile    string `name:"json-file" aliases:"body" help:"Complete Workspace JSON, inline or @file"`
}

type TagManagerWorkspacesListCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *TagManagerWorkspacesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	var items []*tagmanager.Workspace
	next := c.PageToken
	for {
		call := svc.Accounts.Containers.Workspaces.List(tagManagerContainerPath(c.Account, c.Container)).Context(ctx)
		if next != "" {
			call = call.PageToken(next)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return callErr
		}
		items = append(items, resp.Workspace...)
		next = resp.NextPageToken
		if !c.All || next == "" {
			break
		}
	}
	return writeTagManagerList(ctx, "workspaces", items, next, c.FailEmpty, tagManagerWorkspaceRow)
}

type TagManagerWorkspacesGetCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
}

func (c *TagManagerWorkspacesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Workspaces.Get(tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "workspace", item)
}

type TagManagerWorkspacesCreateCmd struct {
	Account                     string `arg:"" name:"account"`
	Container                   string `arg:"" name:"container"`
	TagManagerWorkspaceMutation `embed:""`
}

func (c *TagManagerWorkspacesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body, err := c.body()
	if err != nil {
		return err
	}
	if dryRunErr := marketingDryRunExit(ctx, flags, "tagmanager.workspaces.create", map[string]any{"parent": tagManagerContainerPath(c.Account, c.Container), "workspace": body}); dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Workspaces.Create(tagManagerContainerPath(c.Account, c.Container), body).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "workspace", item)
}

func (c *TagManagerWorkspacesCreateCmd) body() (*tagmanager.Workspace, error) {
	out := &tagmanager.Workspace{Name: strings.TrimSpace(c.Name), Description: c.Description}
	if strings.TrimSpace(c.JSONFile) != "" {
		if err := decodeTagManagerJSON(c.JSONFile, out); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(out.Name) == "" {
		return nil, usage("workspace name is required")
	}
	return out, nil
}

type TagManagerWorkspacesUpdateCmd struct {
	Account                     string `arg:"" name:"account"`
	Container                   string `arg:"" name:"container"`
	Workspace                   string `arg:"" name:"workspace"`
	TagManagerWorkspaceMutation `embed:""`
}

func (c *TagManagerWorkspacesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body, err := c.body()
	if err != nil {
		return err
	}
	path := tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)
	if dryRunErr := marketingDryRunExit(ctx, flags, "tagmanager.workspaces.update", map[string]any{"path": path, "workspace": body}); dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Workspaces.Update(path, body).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "workspace", item)
}

func (c *TagManagerWorkspacesUpdateCmd) body() (*tagmanager.Workspace, error) {
	out := &tagmanager.Workspace{Name: strings.TrimSpace(c.Name), Description: c.Description}
	if strings.TrimSpace(c.JSONFile) != "" {
		if err := decodeTagManagerJSON(c.JSONFile, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type TagManagerWorkspacesDeleteCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
}

func (c *TagManagerWorkspacesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)
	if err := marketingDryRunAndConfirmDestructive(ctx, flags, "tagmanager.workspaces.delete", map[string]any{"path": path}, "delete GTM workspace "+path); err != nil {
		return err
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	if err := svc.Accounts.Containers.Workspaces.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("deleted", true), kv("path", path))
}

type TagManagerWorkspacesStatusCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
}

func (c *TagManagerWorkspacesStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Workspaces.GetStatus(tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "workspace_status", item)
}

type TagManagerWorkspacesSyncCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
}

func (c *TagManagerWorkspacesSyncCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)
	if err := marketingDryRunExit(ctx, flags, "tagmanager.workspaces.sync", map[string]any{"path": path}); err != nil {
		return err
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Workspaces.Sync(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "workspace_sync", item)
}

type TagManagerWorkspaceCreateVersionCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
	Name      string `name:"name"`
	Notes     string `name:"notes"`
}

func (c *TagManagerWorkspaceCreateVersionCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)
	if err := marketingDryRunExit(ctx, flags, "tagmanager.workspaces.create-version", map[string]any{"path": path, "name": c.Name, "notes": c.Notes}); err != nil {
		return err
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Workspaces.CreateVersion(path, &tagmanager.CreateContainerVersionRequestVersionOptions{
		Name:  strings.TrimSpace(c.Name),
		Notes: c.Notes,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "container_version", item)
}

type TagManagerResourceMutation struct {
	Name     string `name:"name"`
	Type     string `name:"type"`
	Notes    string `name:"notes"`
	JSONFile string `name:"json-file" aliases:"body" help:"Complete Tag/Trigger/Variable JSON, inline or @file"`
}

type TagManagerTagsListCmd struct {
	TagManagerResourceListCmd `embed:""`
}

func (c *TagManagerTagsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "tags"
	return c.TagManagerResourceListCmd.Run(ctx, flags)
}

type TagManagerTagsGetCmd struct {
	TagManagerResourceGetCmd `embed:""`
}

func (c *TagManagerTagsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "tags"
	return c.TagManagerResourceGetCmd.Run(ctx, flags)
}

type TagManagerTagsCreateCmd struct {
	TagManagerResourceCreateCmd `embed:""`
}

func (c *TagManagerTagsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "tags"
	return c.TagManagerResourceCreateCmd.Run(ctx, flags)
}

type TagManagerTagsUpdateCmd struct {
	TagManagerResourceUpdateCmd `embed:""`
}

func (c *TagManagerTagsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "tags"
	return c.TagManagerResourceUpdateCmd.Run(ctx, flags)
}

type TagManagerTagsDeleteCmd struct {
	TagManagerResourceDeleteCmd `embed:""`
}

func (c *TagManagerTagsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "tags"
	return c.TagManagerResourceDeleteCmd.Run(ctx, flags)
}

type TagManagerTriggersListCmd struct {
	TagManagerResourceListCmd `embed:""`
}

func (c *TagManagerTriggersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "triggers"
	return c.TagManagerResourceListCmd.Run(ctx, flags)
}

type TagManagerTriggersGetCmd struct {
	TagManagerResourceGetCmd `embed:""`
}

func (c *TagManagerTriggersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "triggers"
	return c.TagManagerResourceGetCmd.Run(ctx, flags)
}

type TagManagerTriggersCreateCmd struct {
	TagManagerResourceCreateCmd `embed:""`
}

func (c *TagManagerTriggersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "triggers"
	return c.TagManagerResourceCreateCmd.Run(ctx, flags)
}

type TagManagerTriggersUpdateCmd struct {
	TagManagerResourceUpdateCmd `embed:""`
}

func (c *TagManagerTriggersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "triggers"
	return c.TagManagerResourceUpdateCmd.Run(ctx, flags)
}

type TagManagerTriggersDeleteCmd struct {
	TagManagerResourceDeleteCmd `embed:""`
}

func (c *TagManagerTriggersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "triggers"
	return c.TagManagerResourceDeleteCmd.Run(ctx, flags)
}

type TagManagerVariablesListCmd struct {
	TagManagerResourceListCmd `embed:""`
}

func (c *TagManagerVariablesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "variables"
	return c.TagManagerResourceListCmd.Run(ctx, flags)
}

type TagManagerVariablesGetCmd struct {
	TagManagerResourceGetCmd `embed:""`
}

func (c *TagManagerVariablesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "variables"
	return c.TagManagerResourceGetCmd.Run(ctx, flags)
}

type TagManagerVariablesCreateCmd struct {
	TagManagerResourceCreateCmd `embed:""`
}

func (c *TagManagerVariablesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "variables"
	return c.TagManagerResourceCreateCmd.Run(ctx, flags)
}

type TagManagerVariablesUpdateCmd struct {
	TagManagerResourceUpdateCmd `embed:""`
}

func (c *TagManagerVariablesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "variables"
	return c.TagManagerResourceUpdateCmd.Run(ctx, flags)
}

type TagManagerVariablesDeleteCmd struct {
	TagManagerResourceDeleteCmd `embed:""`
}

func (c *TagManagerVariablesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	c.Kind = "variables"
	return c.TagManagerResourceDeleteCmd.Run(ctx, flags)
}

type TagManagerTagsCmd struct {
	List   TagManagerTagsListCmd   `cmd:"" default:"withargs" aliases:"ls"`
	Get    TagManagerTagsGetCmd    `cmd:"" name:"get" aliases:"info,show"`
	Create TagManagerTagsCreateCmd `cmd:"" name:"create"`
	Update TagManagerTagsUpdateCmd `cmd:"" name:"update"`
	Delete TagManagerTagsDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove"`
}

type TagManagerTriggersCmd struct {
	List   TagManagerTriggersListCmd   `cmd:"" default:"withargs" aliases:"ls"`
	Get    TagManagerTriggersGetCmd    `cmd:"" name:"get" aliases:"info,show"`
	Create TagManagerTriggersCreateCmd `cmd:"" name:"create"`
	Update TagManagerTriggersUpdateCmd `cmd:"" name:"update"`
	Delete TagManagerTriggersDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove"`
}

type TagManagerVariablesCmd struct {
	List   TagManagerVariablesListCmd   `cmd:"" default:"withargs" aliases:"ls"`
	Get    TagManagerVariablesGetCmd    `cmd:"" name:"get" aliases:"info,show"`
	Create TagManagerVariablesCreateCmd `cmd:"" name:"create"`
	Update TagManagerVariablesUpdateCmd `cmd:"" name:"update"`
	Delete TagManagerVariablesDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove"`
}

type TagManagerResourceListCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
	Kind      string `name:"kind" hidden:"" default:""`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *TagManagerResourceListCmd) Run(ctx context.Context, flags *RootFlags) error {
	kind := strings.TrimSpace(c.Kind)
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	parent := tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)
	switch kind {
	case "tags":
		var items []*tagmanager.Tag
		next := c.PageToken
		for {
			call := svc.Accounts.Containers.Workspaces.Tags.List(parent).Context(ctx)
			if next != "" {
				call = call.PageToken(next)
			}
			resp, callErr := call.Do()
			if callErr != nil {
				return callErr
			}
			items = append(items, resp.Tag...)
			next = resp.NextPageToken
			if !c.All || next == "" {
				break
			}
		}
		return writeTagManagerList(ctx, kind, items, next, c.FailEmpty, tagManagerTagRow)
	case "triggers":
		var items []*tagmanager.Trigger
		next := c.PageToken
		for {
			call := svc.Accounts.Containers.Workspaces.Triggers.List(parent).Context(ctx)
			if next != "" {
				call = call.PageToken(next)
			}
			resp, callErr := call.Do()
			if callErr != nil {
				return callErr
			}
			items = append(items, resp.Trigger...)
			next = resp.NextPageToken
			if !c.All || next == "" {
				break
			}
		}
		return writeTagManagerList(ctx, kind, items, next, c.FailEmpty, tagManagerTriggerRow)
	case "variables":
		var items []*tagmanager.Variable
		next := c.PageToken
		for {
			call := svc.Accounts.Containers.Workspaces.Variables.List(parent).Context(ctx)
			if next != "" {
				call = call.PageToken(next)
			}
			resp, callErr := call.Do()
			if callErr != nil {
				return callErr
			}
			items = append(items, resp.Variable...)
			next = resp.NextPageToken
			if !c.All || next == "" {
				break
			}
		}
		return writeTagManagerList(ctx, kind, items, next, c.FailEmpty, tagManagerVariableRow)
	default:
		return usage("resource kind must be tags, triggers, or variables")
	}
}

type TagManagerResourceGetCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
	Resource  string `arg:"" name:"resource"`
	Kind      string `name:"kind" hidden:"" default:""`
}

func (c *TagManagerResourceGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	path := tagManagerResourcePath(c.Account, c.Container, c.Workspace, c.Kind, c.Resource)
	switch strings.TrimSpace(c.Kind) {
	case "tags":
		item, callErr := svc.Accounts.Containers.Workspaces.Tags.Get(path).Context(ctx).Do()
		if callErr != nil {
			return callErr
		}
		return writeTagManagerItem(ctx, c.Kind, item)
	case "triggers":
		item, callErr := svc.Accounts.Containers.Workspaces.Triggers.Get(path).Context(ctx).Do()
		if callErr != nil {
			return callErr
		}
		return writeTagManagerItem(ctx, c.Kind, item)
	case "variables":
		item, callErr := svc.Accounts.Containers.Workspaces.Variables.Get(path).Context(ctx).Do()
		if callErr != nil {
			return callErr
		}
		return writeTagManagerItem(ctx, c.Kind, item)
	default:
		return usage("resource kind must be tags, triggers, or variables")
	}
}

type TagManagerResourceCreateCmd struct {
	Account                    string `arg:"" name:"account"`
	Container                  string `arg:"" name:"container"`
	Workspace                  string `arg:"" name:"workspace"`
	Kind                       string `name:"kind" hidden:"" default:""`
	TagManagerResourceMutation `embed:""`
}

func (c *TagManagerResourceCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent := tagManagerWorkspacePath(c.Account, c.Container, c.Workspace)
	dryRunErr := marketingDryRunExit(ctx, flags, "tagmanager."+c.Kind+".create", map[string]any{"parent": parent, "name": c.Name, "type": c.Type, "body": c.JSONFile})
	if dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	return runTagManagerResourceMutation(ctx, svc, c.Kind, true, parent, "", c.TagManagerResourceMutation)
}

func runTagManagerResourceMutation(ctx context.Context, svc *tagmanager.Service, kind string, create bool, parent, path string, mutation TagManagerResourceMutation) error {
	switch strings.TrimSpace(kind) {
	case "tags":
		body := &tagmanager.Tag{Name: mutation.Name, Type: mutation.Type, Notes: mutation.Notes}
		if err := decodeTagManagerJSON(mutation.JSONFile, body); err != nil {
			return err
		}
		var item *tagmanager.Tag
		var err error
		if create {
			item, err = svc.Accounts.Containers.Workspaces.Tags.Create(parent, body).Context(ctx).Do()
		} else {
			item, err = svc.Accounts.Containers.Workspaces.Tags.Update(path, body).Context(ctx).Do()
		}
		if err != nil {
			return err
		}
		return writeTagManagerItem(ctx, kind, item)
	case "triggers":
		body := &tagmanager.Trigger{Name: mutation.Name, Type: mutation.Type, Notes: mutation.Notes}
		if err := decodeTagManagerJSON(mutation.JSONFile, body); err != nil {
			return err
		}
		var item *tagmanager.Trigger
		var err error
		if create {
			item, err = svc.Accounts.Containers.Workspaces.Triggers.Create(parent, body).Context(ctx).Do()
		} else {
			item, err = svc.Accounts.Containers.Workspaces.Triggers.Update(path, body).Context(ctx).Do()
		}
		if err != nil {
			return err
		}
		return writeTagManagerItem(ctx, kind, item)
	case "variables":
		body := &tagmanager.Variable{Name: mutation.Name, Type: mutation.Type, Notes: mutation.Notes}
		if err := decodeTagManagerJSON(mutation.JSONFile, body); err != nil {
			return err
		}
		var item *tagmanager.Variable
		var err error
		if create {
			item, err = svc.Accounts.Containers.Workspaces.Variables.Create(parent, body).Context(ctx).Do()
		} else {
			item, err = svc.Accounts.Containers.Workspaces.Variables.Update(path, body).Context(ctx).Do()
		}
		if err != nil {
			return err
		}
		return writeTagManagerItem(ctx, kind, item)
	default:
		return usage("resource kind must be tags, triggers, or variables")
	}
}

type TagManagerResourceUpdateCmd struct {
	Account                    string `arg:"" name:"account"`
	Container                  string `arg:"" name:"container"`
	Workspace                  string `arg:"" name:"workspace"`
	Resource                   string `arg:"" name:"resource"`
	Kind                       string `name:"kind" hidden:"" default:""`
	TagManagerResourceMutation `embed:""`
}

func (c *TagManagerResourceUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerResourcePath(c.Account, c.Container, c.Workspace, c.Kind, c.Resource)
	dryRunErr := marketingDryRunExit(ctx, flags, "tagmanager."+c.Kind+".update", map[string]any{"path": path, "body": c.JSONFile})
	if dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	return runTagManagerResourceMutation(ctx, svc, c.Kind, false, "", path, c.TagManagerResourceMutation)
}

type TagManagerResourceDeleteCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Workspace string `arg:"" name:"workspace"`
	Resource  string `arg:"" name:"resource"`
	Kind      string `name:"kind" hidden:"" default:""`
}

func (c *TagManagerResourceDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerResourcePath(c.Account, c.Container, c.Workspace, c.Kind, c.Resource)
	if dryRunErr := marketingDryRunAndConfirmDestructive(ctx, flags, "tagmanager."+c.Kind+".delete", map[string]any{"path": path}, "delete GTM "+c.Kind+" "+path); dryRunErr != nil {
		return dryRunErr
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(c.Kind) {
	case "tags":
		err = svc.Accounts.Containers.Workspaces.Tags.Delete(path).Context(ctx).Do()
	case "triggers":
		err = svc.Accounts.Containers.Workspaces.Triggers.Delete(path).Context(ctx).Do()
	case "variables":
		err = svc.Accounts.Containers.Workspaces.Variables.Delete(path).Context(ctx).Do()
	default:
		return usage("resource kind must be tags, triggers, or variables")
	}
	if err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("deleted", true), kv("path", path))
}

type TagManagerVersionsCmd struct {
	List    TagManagerVersionsListCmd    `cmd:"" default:"withargs" aliases:"ls"`
	Get     TagManagerVersionsGetCmd     `cmd:"" name:"get" aliases:"info,show"`
	Delete  TagManagerVersionsDeleteCmd  `cmd:"" name:"delete" aliases:"rm,remove"`
	Publish TagManagerVersionsPublishCmd `cmd:"" name:"publish"`
}

type TagManagerVersionsListCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *TagManagerVersionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	parent := tagManagerContainerPath(c.Account, c.Container)
	var items []*tagmanager.ContainerVersionHeader
	next := c.PageToken
	for {
		call := svc.Accounts.Containers.VersionHeaders.List(parent).Context(ctx)
		if next != "" {
			call = call.PageToken(next)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return callErr
		}
		items = append(items, resp.ContainerVersionHeader...)
		next = resp.NextPageToken
		if !c.All || next == "" {
			break
		}
	}
	return writeTagManagerList(ctx, "versions", items, next, c.FailEmpty, tagManagerVersionHeaderRow)
}

type TagManagerVersionsGetCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Version   string `arg:"" name:"version"`
}

func (c *TagManagerVersionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Accounts.Containers.Versions.Get(tagManagerVersionPath(c.Account, c.Container, c.Version)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "version", item)
}

type TagManagerVersionsDeleteCmd struct {
	Account   string `arg:"" name:"account"`
	Container string `arg:"" name:"container"`
	Version   string `arg:"" name:"version"`
}

func (c *TagManagerVersionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerVersionPath(c.Account, c.Container, c.Version)
	if err := marketingDryRunAndConfirmDestructive(ctx, flags, "tagmanager.versions.delete", map[string]any{"path": path}, "delete GTM version "+path); err != nil {
		return err
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	if err := svc.Accounts.Containers.Versions.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("deleted", true), kv("path", path))
}

type TagManagerVersionsPublishCmd struct {
	Account     string `arg:"" name:"account"`
	Container   string `arg:"" name:"container"`
	Version     string `arg:"" name:"version"`
	Fingerprint string `name:"fingerprint"`
}

func (c *TagManagerVersionsPublishCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := tagManagerVersionPath(c.Account, c.Container, c.Version)
	if err := marketingDryRunAndConfirmDestructive(ctx, flags, "tagmanager.versions.publish", map[string]any{"path": path, "fingerprint": c.Fingerprint}, "publish GTM version "+path); err != nil {
		return err
	}
	svc, err := tagManagerFor(ctx, flags)
	if err != nil {
		return err
	}
	call := svc.Accounts.Containers.Versions.Publish(path).Context(ctx)
	if strings.TrimSpace(c.Fingerprint) != "" {
		call = call.Fingerprint(c.Fingerprint)
	}
	item, err := call.Do()
	if err != nil {
		return err
	}
	return writeTagManagerItem(ctx, "publish", item)
}

func tagManagerFor(ctx context.Context, flags *RootFlags) (*tagmanager.Service, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	return tagManagerService(ctx, account)
}

func tagManagerAccountPath(account string) string {
	account = strings.TrimSpace(account)
	if strings.HasPrefix(account, "accounts/") {
		return account
	}
	return "accounts/" + account
}

func tagManagerContainerPath(account, container string) string {
	return tagManagerAccountPath(account) + "/containers/" + strings.TrimPrefix(strings.TrimSpace(container), "containers/")
}

func tagManagerWorkspacePath(account, container, workspace string) string {
	return tagManagerContainerPath(account, container) + "/workspaces/" + strings.TrimPrefix(strings.TrimSpace(workspace), "workspaces/")
}

func tagManagerResourcePath(account, container, workspace, kind, resource string) string {
	return tagManagerWorkspacePath(account, container, workspace) + "/" + strings.TrimSuffix(strings.TrimSpace(kind), "s") + "s/" + strings.TrimSpace(resource)
}

func tagManagerVersionPath(account, container, version string) string {
	return tagManagerContainerPath(account, container) + "/versions/" + strings.TrimSpace(version)
}

func decodeTagManagerJSON(spec string, dst any) error {
	raw, err := resolveInlineOrFileBytes(spec, strings.NewReader(""))
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid Tag Manager JSON: %w", err)
	}
	return nil
}

func writeTagManagerItem(ctx context.Context, key string, item any) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{key: item})
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return writeTagManagerMap(ctx, map[string]any{key: value})
}

func writeTagManagerList[T any](ctx context.Context, key string, items []T, next string, failEmpty bool, row func(T) map[string]any) error {
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{key: items, "nextPageToken": next}); err != nil {
			return err
		}
		return failEmptyExitIf(failEmpty, len(items) == 0)
	}
	if len(items) == 0 {
		return failEmptyExitIf(failEmpty, true)
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, row(item))
	}
	return writeTagManagerMaps(ctx, rows)
}

func writeTagManagerMap(ctx context.Context, value map[string]any) error {
	return writeTagManagerMaps(ctx, []map[string]any{value})
}

func writeTagManagerMaps(ctx context.Context, rows []map[string]any) error {
	keys := map[string]bool{}
	var columns []string
	for _, row := range rows {
		for key := range row {
			if !keys[key] {
				keys[key] = true
				columns = append(columns, key)
			}
		}
	}
	sort.Strings(columns)
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(columns, "\t"))
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, sanitizeTab(formatGoogleAdsValue(row[column])))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
	return nil
}

func tagManagerAccountRow(item *tagmanager.Account) map[string]any {
	return map[string]any{"account_id": item.AccountId, "name": item.Name, "path": item.Path}
}

func tagManagerContainerRow(item *tagmanager.Container) map[string]any {
	return map[string]any{"account_id": item.AccountId, "container_id": item.ContainerId, "name": item.Name, "public_id": item.PublicId, "path": item.Path}
}

func tagManagerWorkspaceRow(item *tagmanager.Workspace) map[string]any {
	return map[string]any{"workspace_id": item.WorkspaceId, "name": item.Name, "path": item.Path}
}

func tagManagerTagRow(item *tagmanager.Tag) map[string]any {
	return map[string]any{"tag_id": item.TagId, "name": item.Name, "type": item.Type, "path": item.Path}
}

func tagManagerTriggerRow(item *tagmanager.Trigger) map[string]any {
	return map[string]any{"trigger_id": item.TriggerId, "name": item.Name, "type": item.Type, "path": item.Path}
}

func tagManagerVariableRow(item *tagmanager.Variable) map[string]any {
	return map[string]any{"variable_id": item.VariableId, "name": item.Name, "type": item.Type, "path": item.Path}
}

func tagManagerVersionHeaderRow(item *tagmanager.ContainerVersionHeader) map[string]any {
	return map[string]any{"container_version_id": item.ContainerVersionId, "name": item.Name, "deleted": item.Deleted, "path": item.Path}
}
