package cmd

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

var tasksLifecycleOpaqueID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}={0,2}$`)

type TasksListsGetCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Opaque task-list ID or @default; no title lookup"`
}
type TasksListsRenameCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Opaque task-list ID or @default"`
	Title      string `arg:"" name:"title" help:"New list title (up to 1024 characters)"`
}
type TasksListsDeleteCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Opaque non-default task-list ID; deletes ALL contained tasks"`
}
type TasksMoveCmd struct {
	TasklistID            string `arg:"" name:"tasklistId" help:"Source opaque list ID or @default"`
	TaskID                string `arg:"" name:"taskId" help:"Task ID"`
	Parent                string `name:"parent" help:"New parent task ID; omitted means top-level"`
	Previous              string `name:"previous" help:"Previous sibling task ID; omitted means first position"`
	DestinationTasklistID string `name:"destination-tasklist-id" help:"Optional destination list ID in the same connected Google account"`
}
type tasksLifecycleMovePlan struct{ TasklistID, TaskID, Parent, Previous, DestinationTasklistID string }

func tasksLifecycleListID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if strings.EqualFold(id, "default") {
		id = defaultTaskListID
	}
	if id == defaultTaskListID {
		return id, nil
	}
	if len(id) < 16 || !tasksLifecycleOpaqueID.MatchString(id) {
		return "", usage("use an opaque task-list ID or @default, not a title")
	}
	return id, nil
}

func tasksLifecycleTaskID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if !tasksLifecycleOpaqueID.MatchString(id) {
		return "", usage("invalid task ID")
	}
	return id, nil
}

func tasksLifecycleTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 1024 {
		return "", usage("task-list title must contain 1..1024 characters")
	}
	return title, nil
}

func tasksLifecycleService(ctx context.Context, flags *RootFlags) (*tasks.Service, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	return tasksService(ctx, account)
}

func writeTasksLifecycleList(ctx context.Context, item *tasks.TaskList) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tasklist": item})
	}
	ui.FromContext(ctx).Out().Linef("id\t%s", item.Id)
	ui.FromContext(ctx).Out().Linef("title\t%s", item.Title)
	return nil
}

func (c *TasksMoveCmd) plan() (tasksLifecycleMovePlan, error) {
	var plan tasksLifecycleMovePlan
	var err error
	plan.TasklistID, err = tasksLifecycleListID(c.TasklistID)
	if err != nil {
		return plan, err
	}
	plan.TaskID, err = tasksLifecycleTaskID(c.TaskID)
	if err != nil {
		return plan, err
	}
	if c.Parent != "" {
		plan.Parent, err = tasksLifecycleTaskID(c.Parent)
		if err != nil {
			return plan, err
		}
	}
	if c.Previous != "" {
		plan.Previous, err = tasksLifecycleTaskID(c.Previous)
		if err != nil {
			return plan, err
		}
	}
	if c.DestinationTasklistID != "" {
		plan.DestinationTasklistID, err = tasksLifecycleListID(c.DestinationTasklistID)
		if err != nil {
			return plan, err
		}
	}
	if plan.Parent == plan.TaskID || plan.Previous == plan.TaskID || plan.Parent != "" && plan.Parent == plan.Previous {
		return plan, usage("task, parent and previous sibling must be distinct")
	}
	return plan, nil
}

func (c *TasksListsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	id, err := tasksLifecycleListID(c.TasklistID)
	if err != nil {
		return err
	}
	svc, err := tasksLifecycleService(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Tasklists.Get(id).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTasksLifecycleList(ctx, item)
}

// Run renames a task list via a partial Patch limited to the title field.
func (c *TasksListsRenameCmd) Run(ctx context.Context, flags *RootFlags) error {
	id, err := tasksLifecycleListID(c.TasklistID)
	if err != nil {
		return err
	}
	title, err := tasksLifecycleTitle(c.Title)
	if err != nil {
		return err
	}
	if guardErr := marketingDryRunExit(ctx, flags, "tasks.lists.rename", map[string]any{
		"tasklist_id": id,
		"title":       title,
	}); guardErr != nil {
		return guardErr
	}
	ctx = googleapi.WithoutRetries(ctx)
	svc, err := tasksLifecycleService(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Tasklists.Patch(id, &tasks.TaskList{Title: title}).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTasksLifecycleList(ctx, item)
}

// Run moves a task by ID within its tasklist, or across tasklists on the
// same account when a destination tasklist is supplied. Omitting parent
// moves the task to the top level; omitting previous makes it the first
// sibling. The task is addressed directly by ID: no GET fallback and no
// implicit name resolution are performed, and retries are disabled for
// this mutation.
func (c *TasksMoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	plan, err := c.plan()
	if err != nil {
		return err
	}
	if guardErr := marketingDryRunExit(ctx, flags, "tasks.move", map[string]any{
		"tasklist_id":             plan.TasklistID,
		"task_id":                 plan.TaskID,
		"parent":                  plan.Parent,
		"previous":                plan.Previous,
		"destination_tasklist_id": plan.DestinationTasklistID,
	}); guardErr != nil {
		return guardErr
	}
	ctx = googleapi.WithoutRetries(ctx)
	svc, err := tasksLifecycleService(ctx, flags)
	if err != nil {
		return err
	}
	call := svc.Tasks.Move(plan.TasklistID, plan.TaskID).Context(ctx)
	// Only forward explicitly provided query params; empty values would
	// change move semantics (top-level / first sibling) on the server.
	if plan.Parent != "" {
		call = call.Parent(plan.Parent)
	}
	if plan.Previous != "" {
		call = call.Previous(plan.Previous)
	}
	if plan.DestinationTasklistID != "" {
		call = call.DestinationTasklist(plan.DestinationTasklistID)
	}
	item, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"task": item})
	}
	out := ui.FromContext(ctx).Out()
	parent := item.Parent
	if parent == "" {
		parent = "(top-level)"
	}
	out.Linef("Moved task %s", item.Id)
	out.Linef("Title: %s", item.Title)
	out.Linef("Parent: %s", parent)
	out.Linef("Position: %s", item.Position)
	return nil
}

func (c *TasksListsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	id, err := tasksLifecycleListID(c.TasklistID)
	if err != nil {
		return err
	}
	if id == defaultTaskListID {
		return usage("cannot delete the default task list")
	}
	if guardErr := marketingDryRunAndConfirmDestructive(ctx, flags, "tasks.lists.delete",
		map[string]any{"tasklist_id": id},
		"delete task list and ALL contained tasks irreversibly"); guardErr != nil {
		return guardErr
	}
	ctx = googleapi.WithoutRetries(ctx)
	svc, err := tasksLifecycleService(ctx, flags)
	if err != nil {
		return err
	}
	if err = svc.Tasklists.Delete(id).Context(ctx).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"deleted": true, "tasklist_id": id})
	}
	ui.FromContext(ctx).Out().Linef("deleted\t%s", id)
	return nil
}
