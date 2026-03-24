package service

import (
	"context"
	"fmt"

	"github.com/monoes/monoes-agent/internal/workflow"
)

const asanaBaseURL = "https://app.asana.com/api/1.0"

// AsanaNode interacts with the Asana REST API.
// Type: "service.asana"
type AsanaNode struct{}

func (n *AsanaNode) Type() string { return "service.asana" }

func (n *AsanaNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token := strVal(config, "token")
	if token == "" {
		return nil, fmt.Errorf("service.asana: 'token' is required")
	}

	operation := strVal(config, "operation")

	var items []workflow.Item
	var err error

	switch operation {
	case "list_tasks":
		items, err = n.listTasks(ctx, token, config)
	case "get_task":
		items, err = n.getTask(ctx, token, config)
	case "create_task":
		items, err = n.createTask(ctx, token, config)
	case "update_task":
		items, err = n.updateTask(ctx, token, config)
	case "list_projects":
		items, err = n.listProjects(ctx, token, config)
	case "list_workspaces":
		items, err = n.listWorkspaces(ctx, token)
	default:
		return nil, fmt.Errorf("service.asana: unknown operation %q", operation)
	}

	if err != nil {
		return nil, err
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

func (n *AsanaNode) listTasks(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	projectGID := strVal(config, "project_gid")
	workspaceGID := strVal(config, "workspace_gid")
	assignee := strVal(config, "assignee")

	var url string
	if projectGID != "" {
		url = fmt.Sprintf("%s/tasks?project=%s&opt_fields=gid,name,notes,due_on,completed,assignee,created_at,modified_at", asanaBaseURL, projectGID)
	} else if workspaceGID != "" && assignee != "" {
		url = fmt.Sprintf("%s/tasks?workspace=%s&assignee=%s&opt_fields=gid,name,notes,due_on,completed,assignee,created_at,modified_at", asanaBaseURL, workspaceGID, assignee)
	} else {
		return nil, fmt.Errorf("service.asana list_tasks: 'project_gid' or ('workspace_gid' + 'assignee') is required")
	}

	result, err := apiRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.asana list_tasks: %w", err)
	}

	data, _ := result["data"].([]interface{})
	return listToItems(data), nil
}

func (n *AsanaNode) getTask(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	taskGID := strVal(config, "task_gid")
	if taskGID == "" {
		return nil, fmt.Errorf("service.asana: 'task_gid' is required for get_task")
	}
	url := fmt.Sprintf("%s/tasks/%s", asanaBaseURL, taskGID)
	result, err := apiRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.asana get_task: %w", err)
	}
	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		data = result
	}
	return []workflow.Item{workflow.NewItem(data)}, nil
}

func (n *AsanaNode) createTask(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	taskData := map[string]interface{}{}

	if name := strVal(config, "name"); name != "" {
		taskData["name"] = name
	}
	if notes := strVal(config, "notes"); notes != "" {
		taskData["notes"] = notes
	}
	if dueOn := strVal(config, "due_on"); dueOn != "" {
		taskData["due_on"] = dueOn
	}
	if assignee := strVal(config, "assignee"); assignee != "" {
		taskData["assignee"] = assignee
	}
	if projectGID := strVal(config, "project_gid"); projectGID != "" {
		taskData["projects"] = []string{projectGID}
	}
	if workspaceGID := strVal(config, "workspace_gid"); workspaceGID != "" {
		taskData["workspace"] = workspaceGID
	}

	body := map[string]interface{}{"data": taskData}
	url := asanaBaseURL + "/tasks"
	result, err := apiRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.asana create_task: %w", err)
	}
	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		data = result
	}
	return []workflow.Item{workflow.NewItem(data)}, nil
}

func (n *AsanaNode) updateTask(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	taskGID := strVal(config, "task_gid")
	if taskGID == "" {
		return nil, fmt.Errorf("service.asana: 'task_gid' is required for update_task")
	}

	taskData := map[string]interface{}{}
	if name := strVal(config, "name"); name != "" {
		taskData["name"] = name
	}
	if notes := strVal(config, "notes"); notes != "" {
		taskData["notes"] = notes
	}
	if dueOn := strVal(config, "due_on"); dueOn != "" {
		taskData["due_on"] = dueOn
	}
	if assignee := strVal(config, "assignee"); assignee != "" {
		taskData["assignee"] = assignee
	}
	// completed field — check presence explicitly since false is a valid value
	if _, ok := config["completed"]; ok {
		taskData["completed"] = boolVal(config, "completed")
	}

	body := map[string]interface{}{"data": taskData}
	url := fmt.Sprintf("%s/tasks/%s", asanaBaseURL, taskGID)
	result, err := apiRequest(ctx, "PUT", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.asana update_task: %w", err)
	}
	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		data = result
	}
	return []workflow.Item{workflow.NewItem(data)}, nil
}

func (n *AsanaNode) listProjects(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	url := asanaBaseURL + "/projects?opt_fields=gid,name,notes,color,archived,created_at,modified_at"
	if workspaceGID := strVal(config, "workspace_gid"); workspaceGID != "" {
		url += "&workspace=" + workspaceGID
	}
	result, err := apiRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.asana list_projects: %w", err)
	}
	data, _ := result["data"].([]interface{})
	return listToItems(data), nil
}

func (n *AsanaNode) listWorkspaces(ctx context.Context, token string) ([]workflow.Item, error) {
	url := asanaBaseURL + "/workspaces"
	result, err := apiRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.asana list_workspaces: %w", err)
	}
	data, _ := result["data"].([]interface{})
	return listToItems(data), nil
}
