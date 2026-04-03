package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// JiraNode interacts with the Jira REST API v3.
// Type: "service.jira"
type JiraNode struct{}

func (n *JiraNode) Type() string { return "service.jira" }

func (n *JiraNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	baseURL := strVal(config, "base_url")
	if baseURL == "" {
		return nil, fmt.Errorf("service.jira: 'base_url' is required")
	}
	email := strVal(config, "email")
	if email == "" {
		return nil, fmt.Errorf("service.jira: 'email' is required")
	}
	apiToken := strVal(config, "api_token")
	if apiToken == "" {
		return nil, fmt.Errorf("service.jira: 'api_token' is required")
	}

	operation := strVal(config, "operation")

	var items []workflow.Item
	var err error

	switch operation {
	case "get_issue":
		items, err = n.getIssue(ctx, baseURL, email, apiToken, config)
	case "create_issue":
		items, err = n.createIssue(ctx, baseURL, email, apiToken, config)
	case "update_issue":
		items, err = n.updateIssue(ctx, baseURL, email, apiToken, config)
	case "list_issues":
		items, err = n.listIssues(ctx, baseURL, email, apiToken, config)
	case "add_comment":
		items, err = n.addComment(ctx, baseURL, email, apiToken, config)
	case "list_projects":
		items, err = n.listProjects(ctx, baseURL, email, apiToken)
	default:
		return nil, fmt.Errorf("service.jira: unknown operation %q", operation)
	}

	if err != nil {
		return nil, err
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// jiraRequest performs a Jira API call with Basic auth returning a JSON object.
func (n *JiraNode) jiraRequest(ctx context.Context, method, url, email, apiToken string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiToken))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return result, nil
}

func (n *JiraNode) getIssue(ctx context.Context, baseURL, email, apiToken string, config map[string]interface{}) ([]workflow.Item, error) {
	issueKey := strVal(config, "issue_key")
	if issueKey == "" {
		return nil, fmt.Errorf("service.jira: 'issue_key' is required for get_issue")
	}
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", baseURL, issueKey)
	result, err := n.jiraRequest(ctx, "GET", url, email, apiToken, nil)
	if err != nil {
		return nil, fmt.Errorf("service.jira get_issue: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *JiraNode) createIssue(ctx context.Context, baseURL, email, apiToken string, config map[string]interface{}) ([]workflow.Item, error) {
	projectKey := strVal(config, "project_key")
	if projectKey == "" {
		return nil, fmt.Errorf("service.jira: 'project_key' is required for create_issue")
	}
	issueType := strVal(config, "issue_type")
	if issueType == "" {
		issueType = "Task"
	}

	body := map[string]interface{}{
		"fields": map[string]interface{}{
			"project":   map[string]interface{}{"key": projectKey},
			"summary":   strVal(config, "summary"),
			"issuetype": map[string]interface{}{"name": issueType},
			"description": map[string]interface{}{
				"type":    "doc",
				"version": 1,
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": strVal(config, "description"),
							},
						},
					},
				},
			},
		},
	}

	url := baseURL + "/rest/api/3/issue"
	result, err := n.jiraRequest(ctx, "POST", url, email, apiToken, body)
	if err != nil {
		return nil, fmt.Errorf("service.jira create_issue: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *JiraNode) updateIssue(ctx context.Context, baseURL, email, apiToken string, config map[string]interface{}) ([]workflow.Item, error) {
	issueKey := strVal(config, "issue_key")
	if issueKey == "" {
		return nil, fmt.Errorf("service.jira: 'issue_key' is required for update_issue")
	}

	fields := map[string]interface{}{}
	if summary := strVal(config, "summary"); summary != "" {
		fields["summary"] = summary
	}
	if description := strVal(config, "description"); description != "" {
		fields["description"] = map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []interface{}{
				map[string]interface{}{
					"type": "paragraph",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": description,
						},
					},
				},
			},
		}
	}

	body := map[string]interface{}{"fields": fields}
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", baseURL, issueKey)
	result, err := n.jiraRequest(ctx, "PUT", url, email, apiToken, body)
	if err != nil {
		return nil, fmt.Errorf("service.jira update_issue: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *JiraNode) listIssues(ctx context.Context, baseURL, email, apiToken string, config map[string]interface{}) ([]workflow.Item, error) {
	jql := strVal(config, "jql")
	if jql == "" {
		// Build JQL from project_key and status
		projectKey := strVal(config, "project_key")
		if projectKey != "" {
			jql = fmt.Sprintf("project = %s", projectKey)
		}
		if status := strVal(config, "status"); status != "" {
			if jql != "" {
				jql += " AND "
			}
			jql += fmt.Sprintf("status = \"%s\"", status)
		}
	}

	body := map[string]interface{}{
		"jql":        jql,
		"maxResults": 100,
		"fields":     []string{"summary", "status", "assignee", "priority", "issuetype", "created", "updated", "description"},
	}

	url := baseURL + "/rest/api/3/search"
	result, err := n.jiraRequest(ctx, "POST", url, email, apiToken, body)
	if err != nil {
		return nil, fmt.Errorf("service.jira list_issues: %w", err)
	}

	issues, _ := result["issues"].([]interface{})
	return listToItems(issues), nil
}

func (n *JiraNode) addComment(ctx context.Context, baseURL, email, apiToken string, config map[string]interface{}) ([]workflow.Item, error) {
	issueKey := strVal(config, "issue_key")
	if issueKey == "" {
		return nil, fmt.Errorf("service.jira: 'issue_key' is required for add_comment")
	}
	comment := strVal(config, "comment")

	body := map[string]interface{}{
		"body": map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []interface{}{
				map[string]interface{}{
					"type": "paragraph",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": comment,
						},
					},
				},
			},
		},
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", baseURL, issueKey)
	result, err := n.jiraRequest(ctx, "POST", url, email, apiToken, body)
	if err != nil {
		return nil, fmt.Errorf("service.jira add_comment: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *JiraNode) listProjects(ctx context.Context, baseURL, email, apiToken string) ([]workflow.Item, error) {
	url := baseURL + "/rest/api/3/project/search?maxResults=100"
	result, err := n.jiraRequest(ctx, "GET", url, email, apiToken, nil)
	if err != nil {
		return nil, fmt.Errorf("service.jira list_projects: %w", err)
	}

	values, _ := result["values"].([]interface{})
	return listToItems(values), nil
}
