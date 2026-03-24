package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/monoes/monoes-agent/internal/workflow"
)

const (
	notionBaseURL = "https://api.notion.com/v1"
	notionVersion = "2022-06-28"
)

// NotionNode interacts with the Notion API.
// Type: "service.notion"
type NotionNode struct{}

func (n *NotionNode) Type() string { return "service.notion" }

func (n *NotionNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token := strVal(config, "token")
	if token == "" {
		return nil, fmt.Errorf("service.notion: 'token' is required")
	}

	operation := strVal(config, "operation")

	var items []workflow.Item
	var err error

	switch operation {
	case "get_page":
		items, err = n.getPage(ctx, token, config)
	case "create_page":
		items, err = n.createPage(ctx, token, config)
	case "update_page":
		items, err = n.updatePage(ctx, token, config)
	case "query_database":
		items, err = n.queryDatabase(ctx, token, config)
	case "get_database":
		items, err = n.getDatabase(ctx, token, config)
	case "create_database":
		items, err = n.createDatabase(ctx, token, config)
	case "append_blocks":
		items, err = n.appendBlocks(ctx, token, config)
	default:
		return nil, fmt.Errorf("service.notion: unknown operation %q", operation)
	}

	if err != nil {
		return nil, err
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// notionRequest performs a Notion API call returning a JSON object.
func (n *NotionNode) notionRequest(ctx context.Context, method, url, token string, body interface{}) (map[string]interface{}, error) {
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

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionVersion)
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

	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return result, nil
}

func (n *NotionNode) getPage(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	pageID := strVal(config, "page_id")
	if pageID == "" {
		return nil, fmt.Errorf("service.notion: 'page_id' is required for get_page")
	}
	url := fmt.Sprintf("%s/pages/%s", notionBaseURL, pageID)
	result, err := n.notionRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.notion get_page: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *NotionNode) createPage(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	parentID := strVal(config, "parent_id")
	if parentID == "" {
		// fall back to database_id as parent
		parentID = strVal(config, "database_id")
	}
	if parentID == "" {
		return nil, fmt.Errorf("service.notion: 'parent_id' is required for create_page")
	}

	body := map[string]interface{}{
		"parent":     map[string]interface{}{"database_id": parentID},
		"properties": mapVal(config, "properties"),
	}
	if children := sliceVal(config, "children"); len(children) > 0 {
		body["children"] = children
	}

	url := notionBaseURL + "/pages"
	result, err := n.notionRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.notion create_page: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *NotionNode) updatePage(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	pageID := strVal(config, "page_id")
	if pageID == "" {
		return nil, fmt.Errorf("service.notion: 'page_id' is required for update_page")
	}
	body := map[string]interface{}{
		"properties": mapVal(config, "properties"),
	}
	url := fmt.Sprintf("%s/pages/%s", notionBaseURL, pageID)
	result, err := n.notionRequest(ctx, "PATCH", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.notion update_page: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *NotionNode) queryDatabase(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	dbID := strVal(config, "database_id")
	if dbID == "" {
		return nil, fmt.Errorf("service.notion: 'database_id' is required for query_database")
	}

	body := map[string]interface{}{}
	if filter := mapVal(config, "filter"); filter != nil {
		body["filter"] = filter
	}
	if sorts := sliceVal(config, "sorts"); len(sorts) > 0 {
		body["sorts"] = sorts
	}
	if ps := intVal(config, "page_size"); ps > 0 {
		body["page_size"] = ps
	}

	url := fmt.Sprintf("%s/databases/%s/query", notionBaseURL, dbID)
	result, err := n.notionRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.notion query_database: %w", err)
	}

	results, _ := result["results"].([]interface{})
	return listToItems(results), nil
}

func (n *NotionNode) getDatabase(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	dbID := strVal(config, "database_id")
	if dbID == "" {
		return nil, fmt.Errorf("service.notion: 'database_id' is required for get_database")
	}
	url := fmt.Sprintf("%s/databases/%s", notionBaseURL, dbID)
	result, err := n.notionRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.notion get_database: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *NotionNode) createDatabase(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	parentID := strVal(config, "parent_id")
	if parentID == "" {
		return nil, fmt.Errorf("service.notion: 'parent_id' is required for create_database")
	}

	body := map[string]interface{}{
		"parent":     map[string]interface{}{"page_id": parentID},
		"properties": mapVal(config, "properties"),
	}

	url := notionBaseURL + "/databases"
	result, err := n.notionRequest(ctx, "POST", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.notion create_database: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

func (n *NotionNode) appendBlocks(ctx context.Context, token string, config map[string]interface{}) ([]workflow.Item, error) {
	pageID := strVal(config, "page_id")
	if pageID == "" {
		return nil, fmt.Errorf("service.notion: 'page_id' is required for append_blocks")
	}
	children := sliceVal(config, "children")
	if len(children) == 0 {
		return nil, fmt.Errorf("service.notion: 'children' is required for append_blocks")
	}

	body := map[string]interface{}{"children": children}
	url := fmt.Sprintf("%s/blocks/%s/children", notionBaseURL, pageID)
	result, err := n.notionRequest(ctx, "PATCH", url, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.notion append_blocks: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}
