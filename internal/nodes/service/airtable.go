package service

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

const airtableBaseURL = "https://api.airtable.com/v0"

// AirtableNode interacts with the Airtable REST API.
// Type: "service.airtable"
type AirtableNode struct{}

func (n *AirtableNode) Type() string { return "service.airtable" }

func (n *AirtableNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token := strVal(config, "token")
	if token == "" {
		return nil, fmt.Errorf("service.airtable: 'token' is required")
	}
	baseID := strVal(config, "base_id")
	if baseID == "" {
		return nil, fmt.Errorf("service.airtable: 'base_id' is required")
	}
	table := strVal(config, "table")
	if table == "" {
		return nil, fmt.Errorf("service.airtable: 'table' is required")
	}

	baseURL := fmt.Sprintf("%s/%s/%s", airtableBaseURL, baseID, url.PathEscape(table))
	operation := strVal(config, "operation")

	var items []workflow.Item
	var err error

	switch operation {
	case "list_records":
		items, err = n.listRecords(ctx, token, baseURL, config)
	case "get_record":
		items, err = n.getRecord(ctx, token, baseURL, config)
	case "create_record":
		items, err = n.createRecord(ctx, token, baseURL, config)
	case "update_record":
		items, err = n.updateRecord(ctx, token, baseURL, config)
	case "delete_record":
		items, err = n.deleteRecord(ctx, token, baseURL, config)
	default:
		return nil, fmt.Errorf("service.airtable: unknown operation %q", operation)
	}

	if err != nil {
		return nil, err
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

func (n *AirtableNode) listRecords(ctx context.Context, token, baseURL string, config map[string]interface{}) ([]workflow.Item, error) {
	reqURL := baseURL + "?"
	params := url.Values{}
	if ff := strVal(config, "filter_formula"); ff != "" {
		params.Set("filterByFormula", ff)
	}
	if mr := intVal(config, "max_records"); mr > 0 {
		params.Set("maxRecords", fmt.Sprintf("%d", mr))
	}
	if view := strVal(config, "view"); view != "" {
		params.Set("view", view)
	}
	reqURL = baseURL + "?" + params.Encode()

	result, err := apiRequest(ctx, "GET", reqURL, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.airtable list_records: %w", err)
	}

	records, _ := result["records"].([]interface{})
	items := make([]workflow.Item, 0, len(records))
	for _, r := range records {
		rec, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		fields, _ := rec["fields"].(map[string]interface{})
		if fields == nil {
			fields = map[string]interface{}{}
		}
		fields["id"] = rec["id"]
		fields["createdTime"] = rec["createdTime"]
		items = append(items, workflow.NewItem(fields))
	}
	return items, nil
}

func (n *AirtableNode) getRecord(ctx context.Context, token, baseURL string, config map[string]interface{}) ([]workflow.Item, error) {
	recordID := strVal(config, "record_id")
	if recordID == "" {
		return nil, fmt.Errorf("service.airtable: 'record_id' is required for get_record")
	}
	reqURL := baseURL + "/" + url.PathEscape(recordID)
	result, err := apiRequest(ctx, "GET", reqURL, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.airtable get_record: %w", err)
	}
	return []workflow.Item{workflow.NewItem(airtableRecordToMap(result))}, nil
}

func (n *AirtableNode) createRecord(ctx context.Context, token, baseURL string, config map[string]interface{}) ([]workflow.Item, error) {
	fields := mapVal(config, "fields")
	body := map[string]interface{}{"fields": fields}
	result, err := apiRequest(ctx, "POST", baseURL, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.airtable create_record: %w", err)
	}
	return []workflow.Item{workflow.NewItem(airtableRecordToMap(result))}, nil
}

func (n *AirtableNode) updateRecord(ctx context.Context, token, baseURL string, config map[string]interface{}) ([]workflow.Item, error) {
	recordID := strVal(config, "record_id")
	if recordID == "" {
		return nil, fmt.Errorf("service.airtable: 'record_id' is required for update_record")
	}
	fields := mapVal(config, "fields")
	body := map[string]interface{}{"fields": fields}
	reqURL := baseURL + "/" + url.PathEscape(recordID)
	result, err := apiRequest(ctx, "PATCH", reqURL, token, body)
	if err != nil {
		return nil, fmt.Errorf("service.airtable update_record: %w", err)
	}
	return []workflow.Item{workflow.NewItem(airtableRecordToMap(result))}, nil
}

func (n *AirtableNode) deleteRecord(ctx context.Context, token, baseURL string, config map[string]interface{}) ([]workflow.Item, error) {
	recordID := strVal(config, "record_id")
	if recordID == "" {
		return nil, fmt.Errorf("service.airtable: 'record_id' is required for delete_record")
	}
	reqURL := baseURL + "/" + url.PathEscape(recordID)
	result, err := apiRequest(ctx, "DELETE", reqURL, token, nil)
	if err != nil {
		return nil, fmt.Errorf("service.airtable delete_record: %w", err)
	}
	return []workflow.Item{workflow.NewItem(result)}, nil
}

// airtableRecordToMap flattens a record's fields into a map with id and createdTime included.
func airtableRecordToMap(rec map[string]interface{}) map[string]interface{} {
	fields, _ := rec["fields"].(map[string]interface{})
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["id"] = rec["id"]
	fields["createdTime"] = rec["createdTime"]
	return fields
}
