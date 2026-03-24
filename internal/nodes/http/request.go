package httpnodes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// RequestNode performs HTTP requests.
// Type: "http.request"
type RequestNode struct{}

func (n *RequestNode) Type() string { return "http.request" }

func (n *RequestNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	method, _ := config["method"].(string)
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	rawURL, _ := config["url"].(string)
	if rawURL == "" {
		return nil, fmt.Errorf("http.request: 'url' is required")
	}

	timeoutSecs := 30
	if v, ok := config["timeout_seconds"].(float64); ok {
		timeoutSecs = int(v)
	}

	followRedirects := true
	if v, ok := config["follow_redirects"].(bool); ok {
		followRedirects = v
	}

	responseFormat, _ := config["response_format"].(string)
	if responseFormat == "" {
		responseFormat = "json"
	}

	bodyType, _ := config["body_type"].(string)
	if bodyType == "" {
		bodyType = "none"
	}

	authType, _ := config["auth_type"].(string)
	if authType == "" {
		authType = "none"
	}

	paginationType, _ := config["pagination_type"].(string)
	if paginationType == "" {
		paginationType = "none"
	}

	pageSize := 100
	if v, ok := config["page_size"].(float64); ok {
		pageSize = int(v)
	}
	pageField, _ := config["page_field"].(string)
	if pageField == "" {
		pageField = "page"
	}

	transport := &http.Transport{}
	client := &http.Client{
		Timeout:   time.Duration(timeoutSecs) * time.Second,
		Transport: transport,
	}
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	items := input.Items
	if len(items) == 0 {
		items = []workflow.Item{{JSON: map[string]interface{}{}}}
	}

	var mainItems []workflow.Item
	var errorItems []workflow.Item

	for _, item := range items {
		if paginationType == "offset" {
			pageItems, pageErrors := n.executePaginated(ctx, client, method, rawURL, config, authType, bodyType, responseFormat, pageField, pageSize, item)
			mainItems = append(mainItems, pageItems...)
			errorItems = append(errorItems, pageErrors...)
		} else {
			resp, errItem := n.executeRequest(ctx, client, method, rawURL, config, authType, bodyType, responseFormat, item, -1)
			if errItem != nil {
				errorItems = append(errorItems, *errItem)
			} else {
				mainItems = append(mainItems, resp)
			}
		}
	}

	var outputs []workflow.NodeOutput
	if len(mainItems) > 0 {
		outputs = append(outputs, workflow.NodeOutput{Handle: "main", Items: mainItems})
	}
	if len(errorItems) > 0 {
		outputs = append(outputs, workflow.NodeOutput{Handle: "error", Items: errorItems})
	}
	return outputs, nil
}

func (n *RequestNode) executePaginated(
	ctx context.Context,
	client *http.Client,
	method, rawURL string,
	config map[string]interface{},
	authType, bodyType, responseFormat string,
	pageField string,
	pageSize int,
	inputItem workflow.Item,
) ([]workflow.Item, []workflow.Item) {
	var allMain []workflow.Item
	var allErrors []workflow.Item

	for page := 0; ; page++ {
		resp, errItem := n.executeRequest(ctx, client, method, rawURL, config, authType, bodyType, responseFormat, inputItem, page)
		if errItem != nil {
			allErrors = append(allErrors, *errItem)
			break
		}

		// Check if body is empty array
		body, ok := resp.JSON["body"]
		if !ok {
			break
		}
		switch v := body.(type) {
		case []interface{}:
			if len(v) == 0 {
				break
			}
			for _, elem := range v {
				m, ok2 := elem.(map[string]interface{})
				if !ok2 {
					m = map[string]interface{}{"value": elem}
				}
				allMain = append(allMain, workflow.NewItem(m))
			}
			if len(v) < pageSize {
				return allMain, allErrors
			}
		default:
			allMain = append(allMain, resp)
			return allMain, allErrors
		}
	}
	return allMain, allErrors
}

func (n *RequestNode) executeRequest(
	ctx context.Context,
	client *http.Client,
	method, rawURL string,
	config map[string]interface{},
	authType, bodyType, responseFormat string,
	inputItem workflow.Item,
	page int,
) (workflow.Item, *workflow.Item) {
	// Build URL
	u, err := url.Parse(rawURL)
	if err != nil {
		errItem := workflow.NewItem(map[string]interface{}{"error": err.Error()})
		return workflow.Item{}, &errItem
	}

	// Query params
	q := u.Query()
	if qp, ok := config["query_params"].(map[string]interface{}); ok {
		for k, v := range qp {
			q.Set(k, fmt.Sprintf("%v", v))
		}
	}
	// API key in query
	if authType == "api_key" {
		apiKeyIn, _ := config["auth_api_key_in"].(string)
		apiKeyName, _ := config["auth_api_key_name"].(string)
		apiKeyValue, _ := config["auth_api_key_value"].(string)
		if apiKeyIn == "query" && apiKeyName != "" {
			q.Set(apiKeyName, apiKeyValue)
		}
	}
	// Pagination offset
	if page >= 0 {
		pageSize := 100
		if v, ok := config["page_size"].(float64); ok {
			pageSize = int(v)
		}
		pageField, _ := config["page_field"].(string)
		if pageField == "" {
			pageField = "page"
		}
		q.Set(pageField, fmt.Sprintf("%d", page))
		q.Set("per_page", fmt.Sprintf("%d", pageSize))
		_ = pageField
	}
	u.RawQuery = q.Encode()

	// Build body
	var bodyReader io.Reader
	var contentType string

	bodyVal := config["body"]
	if bodyType != "none" && bodyVal != nil {
		switch bodyType {
		case "json":
			var bodyBytes []byte
			if s, ok := bodyVal.(string); ok {
				bodyBytes = []byte(s)
			} else {
				bodyBytes, err = json.Marshal(bodyVal)
				if err != nil {
					errItem := workflow.NewItem(map[string]interface{}{"error": "failed to marshal body: " + err.Error()})
					return workflow.Item{}, &errItem
				}
			}
			bodyReader = bytes.NewReader(bodyBytes)
			contentType = "application/json"
		case "form":
			form := url.Values{}
			if m, ok := bodyVal.(map[string]interface{}); ok {
				for k, v := range m {
					form.Set(k, fmt.Sprintf("%v", v))
				}
			} else if s, ok := bodyVal.(string); ok {
				bodyReader = strings.NewReader(s)
				contentType = "application/x-www-form-urlencoded"
				break
			}
			bodyReader = strings.NewReader(form.Encode())
			contentType = "application/x-www-form-urlencoded"
		case "text":
			if s, ok := bodyVal.(string); ok {
				bodyReader = strings.NewReader(s)
			} else {
				b, _ := json.Marshal(bodyVal)
				bodyReader = bytes.NewReader(b)
			}
			contentType = "text/plain"
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		errItem := workflow.NewItem(map[string]interface{}{"error": err.Error()})
		return workflow.Item{}, &errItem
	}

	// Set content type
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Custom headers
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	// Auth
	switch authType {
	case "basic":
		username, _ := config["auth_username"].(string)
		password, _ := config["auth_password"].(string)
		req.SetBasicAuth(username, password)
	case "bearer":
		token, _ := config["auth_token"].(string)
		req.Header.Set("Authorization", "Bearer "+token)
	case "api_key":
		apiKeyIn, _ := config["auth_api_key_in"].(string)
		apiKeyName, _ := config["auth_api_key_name"].(string)
		apiKeyValue, _ := config["auth_api_key_value"].(string)
		if apiKeyIn == "header" && apiKeyName != "" {
			req.Header.Set(apiKeyName, apiKeyValue)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		errItem := workflow.NewItem(map[string]interface{}{"error": err.Error()})
		return workflow.Item{}, &errItem
	}
	defer resp.Body.Close()

	// Read response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		errItem := workflow.NewItem(map[string]interface{}{"error": "failed to read response: " + err.Error()})
		return workflow.Item{}, &errItem
	}

	// Collect response headers
	respHeaders := make(map[string]interface{})
	for k, vals := range resp.Header {
		if len(vals) == 1 {
			respHeaders[k] = vals[0]
		} else {
			ivals := make([]interface{}, len(vals))
			for i, v := range vals {
				ivals[i] = v
			}
			respHeaders[k] = ivals
		}
	}

	// Parse response body
	var parsedBody interface{}
	switch responseFormat {
	case "json":
		if err := json.Unmarshal(respBytes, &parsedBody); err != nil {
			// Fall back to text
			parsedBody = string(respBytes)
		}
	case "binary":
		parsedBody = base64.StdEncoding.EncodeToString(respBytes)
	default: // "text"
		parsedBody = string(respBytes)
	}

	resultItem := workflow.NewItem(map[string]interface{}{
		"status":  resp.StatusCode,
		"headers": respHeaders,
		"body":    parsedBody,
	})

	// Non-2xx goes to error handle
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflow.Item{}, &resultItem
	}

	return resultItem, nil
}
