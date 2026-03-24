package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// GoogleSheetsNode implements the service.google_sheets node type.
type GoogleSheetsNode struct{}

func (n *GoogleSheetsNode) Type() string { return "service.google_sheets" }

func (n *GoogleSheetsNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	accessToken := strVal(config, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("google_sheets: access_token is required")
	}

	operation := strVal(config, "operation")
	if operation == "" {
		operation = "append_rows"
	}

	// --- create_spreadsheet: create a new spreadsheet and return its metadata ---
	if operation == "create_spreadsheet" {
		title := strVal(config, "title")
		if title == "" {
			title = "Monoes Export"
		}
		meta, err := sheetsCreateSpreadsheet(ctx, accessToken, title)
		if err != nil {
			return nil, fmt.Errorf("google_sheets create_spreadsheet: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{workflow.NewItem(meta)}}}, nil
	}

	// For all other operations we need a spreadsheet_id.
	// If it is "new" (or empty), create one first.
	spreadsheetID := strVal(config, "spreadsheet_id")
	if spreadsheetID == "" || spreadsheetID == "new" {
		title := strVal(config, "title")
		if title == "" {
			title = "Monoes Export"
		}
		meta, err := sheetsCreateSpreadsheet(ctx, accessToken, title)
		if err != nil {
			return nil, fmt.Errorf("google_sheets: auto-create spreadsheet: %w", err)
		}
		if id, ok := meta["spreadsheetId"].(string); ok && id != "" {
			spreadsheetID = id
		} else {
			return nil, fmt.Errorf("google_sheets: could not determine spreadsheetId from create response")
		}
	}

	sheet := strVal(config, "sheet_name")
	if sheet == "" {
		sheet = strVal(config, "sheet")
	}
	if sheet == "" {
		sheet = "Sheet1"
	}

	rangeStr := strVal(config, "range")
	if rangeStr == "" {
		rangeStr = sheet
	} else {
		rangeStr = sheet + "!" + rangeStr
	}

	valueInputOption := strVal(config, "value_input_option")
	if valueInputOption == "" {
		valueInputOption = "RAW"
	}
	useHeaderRow := boolVal(config, "use_header_row")

	baseURL := "https://sheets.googleapis.com/v4/spreadsheets/" + spreadsheetID

	var items []workflow.Item

	switch operation {
	case "read_rows":
		url := baseURL + "/values/" + sheetsEncodeRange(rangeStr)
		data, err := apiRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("google_sheets read_rows: %w", err)
		}
		values, _ := data["values"].([]interface{})
		items = sheetsValuesToItems(values, useHeaderRow)

	case "append_rows":
		values := sheetsExtractValues(config)
		if len(values) == 0 && len(input.Items) > 0 {
			// Auto-build rows from pipeline items.
			values = sheetsItemsToRows(input.Items, true)
		}
		url := baseURL + "/values/" + sheetsEncodeRange(rangeStr) + ":append?valueInputOption=" + valueInputOption
		body := map[string]interface{}{
			"values": values,
		}
		resp, err := sheetsRequest(ctx, "POST", url, accessToken, body)
		if err != nil {
			return nil, fmt.Errorf("google_sheets append_rows: %w", err)
		}
		// Return updated metadata plus original items so they continue flowing.
		result := map[string]interface{}{
			"spreadsheet_id": spreadsheetID,
			"rows_written":   len(values),
			"response":       resp,
		}
		items = []workflow.Item{workflow.NewItem(result)}

	case "update_rows":
		values := sheetsExtractValues(config)
		url := baseURL + "/values/" + sheetsEncodeRange(rangeStr) + "?valueInputOption=" + valueInputOption
		body := map[string]interface{}{
			"range":  rangeStr,
			"values": values,
		}
		resp, err := sheetsRequest(ctx, "PUT", url, accessToken, body)
		if err != nil {
			return nil, fmt.Errorf("google_sheets update_rows: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(resp)}

	case "clear_range":
		url := baseURL + "/values/" + sheetsEncodeRange(rangeStr) + ":clear"
		resp, err := sheetsRequest(ctx, "POST", url, accessToken, map[string]interface{}{})
		if err != nil {
			return nil, fmt.Errorf("google_sheets clear_range: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(resp)}

	default:
		return nil, fmt.Errorf("google_sheets: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// sheetsCreateSpreadsheet creates a new Google Spreadsheet and returns its metadata.
func sheetsCreateSpreadsheet(ctx context.Context, accessToken, title string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"title": title,
		},
	}
	return sheetsRequest(ctx, "POST", "https://sheets.googleapis.com/v4/spreadsheets", accessToken, body)
}

// sheetsItemsToRows converts workflow Items to a [][]interface{} suitable for the Sheets API.
// If withHeader is true, the first row contains the sorted field names as headers.
func sheetsItemsToRows(items []workflow.Item, withHeader bool) [][]interface{} {
	if len(items) == 0 {
		return nil
	}

	// Collect all keys across all items, preserving a stable order.
	keySet := map[string]bool{}
	for _, item := range items {
		for k := range item.JSON {
			keySet[k] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([][]interface{}, 0, len(items)+1)
	if withHeader {
		header := make([]interface{}, len(keys))
		for i, k := range keys {
			header[i] = k
		}
		rows = append(rows, header)
	}
	for _, item := range items {
		row := make([]interface{}, len(keys))
		for i, k := range keys {
			if v, ok := item.JSON[k]; ok && v != nil {
				row[i] = fmt.Sprintf("%v", v)
			} else {
				row[i] = ""
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// sheetsRequest makes an authenticated request to the Google Sheets API.
func sheetsRequest(ctx context.Context, method, url, accessToken string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("google_sheets: marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("google_sheets: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_sheets %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("google_sheets: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google_sheets HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("google_sheets: parsing JSON: %w", err)
	}
	return result, nil
}

// sheetsValuesToItems converts Sheets API value rows to workflow Items.
func sheetsValuesToItems(values []interface{}, useHeaderRow bool) []workflow.Item {
	if len(values) == 0 {
		return nil
	}

	var headers []string
	startRow := 0

	if useHeaderRow {
		if headerRow, ok := values[0].([]interface{}); ok {
			headers = make([]string, len(headerRow))
			for i, h := range headerRow {
				headers[i] = fmt.Sprintf("%v", h)
			}
			startRow = 1
		}
	}

	items := make([]workflow.Item, 0, len(values)-startRow)
	for i := startRow; i < len(values); i++ {
		row, ok := values[i].([]interface{})
		if !ok {
			continue
		}
		data := make(map[string]interface{}, len(row))
		for j, cell := range row {
			var key string
			if useHeaderRow && j < len(headers) {
				key = headers[j]
			} else {
				key = sheetsColumnLetter(j)
			}
			data[key] = cell
		}
		items = append(items, workflow.NewItem(data))
	}
	return items
}

func sheetsColumnLetter(idx int) string {
	result := ""
	idx++
	for idx > 0 {
		idx--
		result = string(rune('A'+idx%26)) + result
		idx /= 26
	}
	return result
}

func sheetsEncodeRange(r string) string {
	out := make([]byte, 0, len(r))
	for i := 0; i < len(r); i++ {
		c := r[i]
		if c == ' ' {
			out = append(out, '%', '2', '0')
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

func sheetsExtractValues(config map[string]interface{}) [][]interface{} {
	raw, ok := config["values"].([]interface{})
	if !ok {
		return nil
	}
	result := make([][]interface{}, 0, len(raw))
	for _, row := range raw {
		if r, ok := row.([]interface{}); ok {
			result = append(result, r)
		}
	}
	return result
}
