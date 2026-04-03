package service

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// SalesforceNode implements the service.salesforce node type.
type SalesforceNode struct{}

func (n *SalesforceNode) Type() string { return "service.salesforce" }

func (n *SalesforceNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	instanceURL := strVal(config, "instance_url")
	if instanceURL == "" {
		return nil, fmt.Errorf("salesforce: instance_url is required")
	}
	accessToken := strVal(config, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("salesforce: access_token is required")
	}
	operation := strVal(config, "operation")
	baseURL := instanceURL + "/services/data/v58.0"

	var items []workflow.Item

	switch operation {
	case "query":
		soql := strVal(config, "soql")
		if soql == "" {
			return nil, fmt.Errorf("salesforce: soql is required for query")
		}
		records, err := salesforceQuery(ctx, baseURL, accessToken, soql)
		if err != nil {
			return nil, err
		}
		items = records

	case "get_record":
		objectType := strVal(config, "object_type")
		recordID := strVal(config, "record_id")
		if objectType == "" || recordID == "" {
			return nil, fmt.Errorf("salesforce: object_type and record_id are required for get_record")
		}
		url := baseURL + "/sobjects/" + objectType + "/" + recordID
		data, err := apiRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "create_record":
		objectType := strVal(config, "object_type")
		if objectType == "" {
			return nil, fmt.Errorf("salesforce: object_type is required for create_record")
		}
		fields := mapVal(config, "fields")
		if fields == nil {
			fields = map[string]interface{}{}
		}
		url := baseURL + "/sobjects/" + objectType
		data, err := apiRequest(ctx, "POST", url, accessToken, fields)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "update_record":
		objectType := strVal(config, "object_type")
		recordID := strVal(config, "record_id")
		if objectType == "" || recordID == "" {
			return nil, fmt.Errorf("salesforce: object_type and record_id are required for update_record")
		}
		fields := mapVal(config, "fields")
		if fields == nil {
			fields = map[string]interface{}{}
		}
		url := baseURL + "/sobjects/" + objectType + "/" + recordID
		_, err := apiRequest(ctx, "PATCH", url, accessToken, fields)
		if err != nil {
			return nil, err
		}
		// PATCH returns 204 no content; return confirmation item
		items = []workflow.Item{workflow.NewItem(map[string]interface{}{
			"id":      recordID,
			"updated": true,
		})}

	case "delete_record":
		objectType := strVal(config, "object_type")
		recordID := strVal(config, "record_id")
		if objectType == "" || recordID == "" {
			return nil, fmt.Errorf("salesforce: object_type and record_id are required for delete_record")
		}
		url := baseURL + "/sobjects/" + objectType + "/" + recordID
		_, err := apiRequest(ctx, "DELETE", url, accessToken, nil)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(map[string]interface{}{
			"id":      recordID,
			"deleted": true,
		})}

	case "describe_object":
		objectType := strVal(config, "object_type")
		if objectType == "" {
			return nil, fmt.Errorf("salesforce: object_type is required for describe_object")
		}
		url := baseURL + "/sobjects/" + objectType + "/describe"
		data, err := apiRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	default:
		return nil, fmt.Errorf("salesforce: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// salesforceQuery executes a SOQL query and paginates through all records.
func salesforceQuery(ctx context.Context, baseURL, accessToken, soql string) ([]workflow.Item, error) {
	var allItems []workflow.Item

	// URL-encode the query
	queryURL := baseURL + "/query?q=" + salesforceURLEncode(soql)

	for queryURL != "" {
		data, err := apiRequest(ctx, "GET", queryURL, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("salesforce query: %w", err)
		}

		records, _ := data["records"].([]interface{})
		for _, r := range records {
			if m, ok := r.(map[string]interface{}); ok {
				allItems = append(allItems, workflow.NewItem(m))
			}
		}

		// Check for next page
		done, _ := data["done"].(bool)
		if done {
			break
		}
		nextURL, _ := data["nextRecordsUrl"].(string)
		if nextURL == "" {
			break
		}
		// nextRecordsUrl is a path like /services/data/v58.0/query/...
		// Extract the base instance URL
		// baseURL is like https://myorg.salesforce.com/services/data/v58.0
		// We need the instance root
		instanceRoot := salesforceInstanceRoot(baseURL)
		queryURL = instanceRoot + nextURL
	}

	return allItems, nil
}

// salesforceURLEncode encodes a string for use in a query parameter.
func salesforceURLEncode(s string) string {
	return url.QueryEscape(s)
}

// salesforceInstanceRoot extracts the instance root URL from a base URL.
// e.g. https://myorg.salesforce.com/services/data/v58.0 -> https://myorg.salesforce.com
func salesforceInstanceRoot(baseURL string) string {
	// Find the third slash (after https://)
	count := 0
	for i, c := range baseURL {
		if c == '/' {
			count++
			if count == 3 {
				return baseURL[:i]
			}
		}
	}
	return baseURL
}
