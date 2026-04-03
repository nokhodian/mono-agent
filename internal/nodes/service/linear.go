package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

const linearGraphQLURL = "https://api.linear.app/graphql"

// GraphQL query constants for each Linear operation.
const (
	linearQueryListIssues = `
query ListIssues($teamId: String, $projectId: String, $filter: IssueFilter, $first: Int) {
  issues(filter: $filter, first: $first) {
    nodes {
      id
      title
      description
      priority
      state { id name }
      team { id name }
      project { id name }
      assignee { id name email }
      createdAt
      updatedAt
    }
  }
}`

	linearQueryGetIssue = `
query GetIssue($id: String!) {
  issue(id: $id) {
    id
    title
    description
    priority
    state { id name }
    team { id name }
    project { id name }
    assignee { id name email }
    createdAt
    updatedAt
  }
}`

	linearMutationCreateIssue = `
mutation CreateIssue($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      title
      description
      priority
      state { id name }
      team { id name }
      createdAt
    }
  }
}`

	linearMutationUpdateIssue = `
mutation UpdateIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue {
      id
      title
      description
      priority
      state { id name }
      updatedAt
    }
  }
}`

	linearQueryListTeams = `
query ListTeams {
  teams {
    nodes {
      id
      name
      key
      description
    }
  }
}`

	linearQueryListProjects = `
query ListProjects($teamId: String) {
  projects(filter: { teams: { id: { eq: $teamId } } }) {
    nodes {
      id
      name
      description
      state
      createdAt
      updatedAt
    }
  }
}`
)

// LinearNode interacts with the Linear GraphQL API.
// Type: "service.linear"
type LinearNode struct{}

func (n *LinearNode) Type() string { return "service.linear" }

func (n *LinearNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token := strVal(config, "token")
	if token == "" {
		return nil, fmt.Errorf("service.linear: 'token' is required")
	}

	operation := strVal(config, "operation")

	var items []workflow.Item
	var err error

	switch operation {
	case "list_issues":
		items, err = n.listIssues(ctx, token, config)
	case "get_issue":
		items, err = n.getIssue(ctx, token, config)
	case "create_issue":
		items, err = n.createIssue(ctx, token, config)
	case "update_issue":
		items, err = n.updateIssue(ctx, token, config)
	case "list_teams":
		items, err = n.listTeams(ctx, token)
	case "list_projects":
		items, err = n.listProjects(ctx, token, config)
	default:
		return nil, fmt.Errorf("service.linear: unknown operation %q", operation)
	}

	if err != nil {
		return nil, err
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// linearGraphQL executes a GraphQL query against the Linear API.
func (n *LinearNode) linearGraphQL(ctx context.Context, token, query string, variables map[string]interface{}) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling GraphQL payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", linearGraphQLURL, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	// Linear uses the token directly without "Bearer" prefix
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP POST %s: %w", linearGraphQLURL, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var gqlResp struct {
		Data   map[string]interface{} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &gqlResp); err != nil {
		return nil, fmt.Errorf("parsing GraphQL response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %s", gqlResp.Errors[0].Message)
	}
	if gqlResp.Data == nil {
		return map[string]interface{}{}, nil
	}
	return gqlResp.Data, nil
}

// nodesFromData extracts a node list from GraphQL data at the given top-level key.
// data["key"]["nodes"] -> []interface{}
func nodesFromData(data map[string]interface{}, key string) []interface{} {
	top, _ := data[key].(map[string]interface{})
	if top == nil {
		return nil
	}
	nodes, _ := top["nodes"].([]interface{})
	return nodes
}

func (n *LinearNode) listIssues(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	filter := mapVal(config, "filter")
	vars := map[string]interface{}{}
	if teamID := strVal(config, "team_id"); teamID != "" {
		if filter == nil {
			filter = map[string]interface{}{}
		}
		filter["team"] = map[string]interface{}{"id": map[string]interface{}{"eq": teamID}}
	}
	if filter != nil {
		vars["filter"] = filter
	}
	vars["first"] = 100

	data, err := n.linearGraphQL(ctx, token, linearQueryListIssues, vars)
	if err != nil {
		return nil, fmt.Errorf("service.linear list_issues: %w", err)
	}
	return listToItems(nodesFromData(data, "issues")), nil
}

func (n *LinearNode) getIssue(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	issueID := strVal(config, "issue_id")
	if issueID == "" {
		return nil, fmt.Errorf("service.linear: 'issue_id' is required for get_issue")
	}
	vars := map[string]interface{}{"id": issueID}
	data, err := n.linearGraphQL(ctx, token, linearQueryGetIssue, vars)
	if err != nil {
		return nil, fmt.Errorf("service.linear get_issue: %w", err)
	}
	issue, _ := data["issue"].(map[string]interface{})
	if issue == nil {
		return []workflow.Item{}, nil
	}
	return []workflow.Item{workflow.NewItem(issue)}, nil
}

func (n *LinearNode) createIssue(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	teamID := strVal(config, "team_id")
	if teamID == "" {
		return nil, fmt.Errorf("service.linear: 'team_id' is required for create_issue")
	}

	input := map[string]interface{}{
		"teamId": teamID,
		"title":  strVal(config, "title"),
	}
	if desc := strVal(config, "description"); desc != "" {
		input["description"] = desc
	}
	if priority := intVal(config, "priority"); priority > 0 {
		input["priority"] = priority
	}
	if stateID := strVal(config, "state_id"); stateID != "" {
		input["stateId"] = stateID
	}
	if projectID := strVal(config, "project_id"); projectID != "" {
		input["projectId"] = projectID
	}

	vars := map[string]interface{}{"input": input}
	data, err := n.linearGraphQL(ctx, token, linearMutationCreateIssue, vars)
	if err != nil {
		return nil, fmt.Errorf("service.linear create_issue: %w", err)
	}
	result, _ := data["issueCreate"].(map[string]interface{})
	if result == nil {
		return []workflow.Item{}, nil
	}
	issue, _ := result["issue"].(map[string]interface{})
	if issue == nil {
		issue = result
	}
	return []workflow.Item{workflow.NewItem(issue)}, nil
}

func (n *LinearNode) updateIssue(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	issueID := strVal(config, "issue_id")
	if issueID == "" {
		return nil, fmt.Errorf("service.linear: 'issue_id' is required for update_issue")
	}

	input := map[string]interface{}{}
	if title := strVal(config, "title"); title != "" {
		input["title"] = title
	}
	if desc := strVal(config, "description"); desc != "" {
		input["description"] = desc
	}
	if priority := intVal(config, "priority"); priority > 0 {
		input["priority"] = priority
	}
	if stateID := strVal(config, "state_id"); stateID != "" {
		input["stateId"] = stateID
	}

	vars := map[string]interface{}{"id": issueID, "input": input}
	data, err := n.linearGraphQL(ctx, token, linearMutationUpdateIssue, vars)
	if err != nil {
		return nil, fmt.Errorf("service.linear update_issue: %w", err)
	}
	result, _ := data["issueUpdate"].(map[string]interface{})
	if result == nil {
		return []workflow.Item{}, nil
	}
	issue, _ := result["issue"].(map[string]interface{})
	if issue == nil {
		issue = result
	}
	return []workflow.Item{workflow.NewItem(issue)}, nil
}

func (n *LinearNode) listTeams(ctx context.Context, token string) ([]workflow.Item, error) {
	data, err := n.linearGraphQL(ctx, token, linearQueryListTeams, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("service.linear list_teams: %w", err)
	}
	return listToItems(nodesFromData(data, "teams")), nil
}

func (n *LinearNode) listProjects(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	vars := map[string]interface{}{}
	if teamID := strVal(config, "team_id"); teamID != "" {
		vars["teamId"] = teamID
	}
	data, err := n.linearGraphQL(ctx, token, linearQueryListProjects, vars)
	if err != nil {
		return nil, fmt.Errorf("service.linear list_projects: %w", err)
	}
	return listToItems(nodesFromData(data, "projects")), nil
}
