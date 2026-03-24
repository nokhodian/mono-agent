package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiRequest makes an authenticated HTTP request and returns a parsed JSON object response.
// method: GET/POST/PUT/PATCH/DELETE
// url: full URL
// token: Bearer token (empty = no auth header added)
// body: request body (marshaled to JSON, nil = no body)
// Returns parsed response body as map[string]interface{}.
// For non-2xx responses, returns error with status code and body.
func apiRequest(ctx context.Context, method, url, token string, body interface{}) (map[string]interface{}, error) {
	req, err := buildRequest(ctx, method, url, token, body)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w (body: %s)", err, string(respBytes))
	}
	return result, nil
}

// apiRequestList is like apiRequest but returns []interface{} for array responses.
func apiRequestList(ctx context.Context, method, url, token string, body interface{}) ([]interface{}, error) {
	req, err := buildRequest(ctx, method, url, token, body)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(respBytes) == 0 {
		return []interface{}{}, nil
	}

	var result []interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON array response: %w (body: %s)", err, string(respBytes))
	}
	return result, nil
}

// buildRequest creates an http.Request with optional bearer token and JSON body.
func buildRequest(ctx context.Context, method, url, token string, body interface{}) (*http.Request, error) {
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

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return req, nil
}

// strVal safely extracts a string from a config map.
func strVal(config map[string]interface{}, key string) string {
	v, _ := config[key].(string)
	return v
}

// intVal safely extracts an int from a config map (JSON numbers are float64).
func intVal(config map[string]interface{}, key string) int {
	switch v := config[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// boolVal safely extracts a bool from a config map.
func boolVal(config map[string]interface{}, key string) bool {
	v, _ := config[key].(bool)
	return v
}

// mapVal safely extracts a map from a config map.
func mapVal(config map[string]interface{}, key string) map[string]interface{} {
	v, _ := config[key].(map[string]interface{})
	return v
}

// sliceVal safely extracts a []interface{} from a config map.
func sliceVal(config map[string]interface{}, key string) []interface{} {
	v, _ := config[key].([]interface{})
	return v
}

// strSliceVal safely extracts a []string from a config map.
func strSliceVal(config map[string]interface{}, key string) []string {
	raw, ok := config[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
