package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

const githubBaseURL = "https://api.github.com"

// GitHubNode interacts with the GitHub REST API v3.
// Type: "service.github"
type GitHubNode struct{}

func (n *GitHubNode) Type() string { return "service.github" }

func (n *GitHubNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token := strVal(config, "token")
	if token == "" {
		return nil, fmt.Errorf("service.github: 'token' is required")
	}

	operation := strVal(config, "operation")
	owner := strVal(config, "owner")
	repo := strVal(config, "repo")

	var items []workflow.Item
	var err error

	switch operation {
	case "list_repos":
		items, err = n.listRepos(ctx, token)
	case "list_issues":
		items, err = n.listIssues(ctx, token, owner, repo, config)
	case "get_issue":
		items, err = n.getIssue(ctx, token, owner, repo, config)
	case "create_issue":
		items, err = n.createIssue(ctx, token, owner, repo, config)
	case "update_issue":
		items, err = n.updateIssue(ctx, token, owner, repo, config)
	case "list_prs":
		items, err = n.listPRs(ctx, token, owner, repo, config)
	case "create_pr":
		items, err = n.createPR(ctx, token, owner, repo, config)
	case "list_releases":
		items, err = n.listReleases(ctx, token, owner, repo)
	case "create_release":
		items, err = n.createRelease(ctx, token, owner, repo, config)
	default:
		return nil, fmt.Errorf("service.github: unknown operation %q", operation)
	}

	if err != nil {
		return nil, err
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// ghRequest performs a GitHub API call returning a single JSON object.
func (n *GitHubNode) ghRequest(ctx context.Context, method, url, token string, body interface{}) (map[string]interface{}, error) {
	req, err := buildRequest(ctx, method, url, token, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

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
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return result, nil
}

// ghRequestList performs a GitHub API call returning a JSON array.
func (n *GitHubNode) ghRequestList(ctx context.Context, method, url, token string, body interface{}) ([]interface{}, error) {
	req, err := buildRequest(ctx, method, url, token, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

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
	var result []interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON array: %w", err)
	}
	return result, nil
}

func (n *GitHubNode) listRepos(ctx context.Context, token string) ([]workflow.Item, error) {
	url := githubBaseURL + "/user/repos?per_page=100"
	list, err := n.ghRequestList(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	return listToItems(list), nil
}

func (n *GitHubNode) listIssues(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues?per_page=100", githubBaseURL, owner, repo)
	if state := strVal(config, "state"); state != "" {
		url += "&state=" + state
	}
	list, err := n.ghRequestList(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	return listToItems(list), nil
}

func (n *GitHubNode) getIssue(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	number := intVal(config, "number")
	if number == 0 {
		return nil, fmt.Errorf("service.github: 'number' is required for get_issue")
	}
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", githubBaseURL, owner, repo, number)
	result, err := n.ghRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *GitHubNode) createIssue(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	body := map[string]interface{}{
		"title": strVal(config, "title"),
		"body":  strVal(config, "body"),
	}
	if labels := strSliceVal(config, "labels"); len(labels) > 0 {
		body["labels"] = labels
	}
	url := fmt.Sprintf("%s/repos/%s/%s/issues", githubBaseURL, owner, repo)
	result, err := n.ghRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, err
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *GitHubNode) updateIssue(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	number := intVal(config, "number")
	if number == 0 {
		return nil, fmt.Errorf("service.github: 'number' is required for update_issue")
	}
	body := map[string]interface{}{}
	if title := strVal(config, "title"); title != "" {
		body["title"] = title
	}
	if b := strVal(config, "body"); b != "" {
		body["body"] = b
	}
	if state := strVal(config, "state"); state != "" {
		body["state"] = state
	}
	if labels := strSliceVal(config, "labels"); len(labels) > 0 {
		body["labels"] = labels
	}
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", githubBaseURL, owner, repo, number)
	result, err := n.ghRequest(ctx, "PATCH", url, token, body)
	if err != nil {
		return nil, err
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *GitHubNode) listPRs(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?per_page=100", githubBaseURL, owner, repo)
	if state := strVal(config, "state"); state != "" {
		url += "&state=" + state
	}
	list, err := n.ghRequestList(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	return listToItems(list), nil
}

func (n *GitHubNode) createPR(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	head := strVal(config, "head")
	base := strVal(config, "base")
	if head == "" {
		return nil, fmt.Errorf("service.github: 'head' is required for create_pr")
	}
	if base == "" {
		return nil, fmt.Errorf("service.github: 'base' is required for create_pr")
	}
	body := map[string]interface{}{
		"title": strVal(config, "title"),
		"body":  strVal(config, "body"),
		"head":  head,
		"base":  base,
	}
	url := fmt.Sprintf("%s/repos/%s/%s/pulls", githubBaseURL, owner, repo)
	result, err := n.ghRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, err
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *GitHubNode) listReleases(ctx context.Context, token, owner, repo string) ([]workflow.Item, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", githubBaseURL, owner, repo)
	list, err := n.ghRequestList(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	return listToItems(list), nil
}

func (n *GitHubNode) createRelease(ctx context.Context, token, owner, repo string, config map[string]interface{}) ([]workflow.Item, error) {
	body := map[string]interface{}{
		"tag_name": strVal(config, "tag_name"),
		"name":     strVal(config, "release_name"),
		"body":     strVal(config, "body"),
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases", githubBaseURL, owner, repo)
	result, err := n.ghRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, err
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

// listToItems converts a []interface{} (each element a map) to []workflow.Item.
func listToItems(list []interface{}) []workflow.Item {
	items := make([]workflow.Item, 0, len(list))
	for _, elem := range list {
		if m, ok := elem.(map[string]interface{}); ok {
			items = append(items, workflow.NewItem(m))
		} else {
			items = append(items, workflow.NewItem(map[string]interface{}{"value": elem}))
		}
	}
	return items
}
