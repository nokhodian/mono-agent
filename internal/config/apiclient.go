package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

const (
	defaultBaseURL     = "http://apiv1.monoes.me"
	defaultTimeout     = 30 * time.Second
	generateTimeout    = 90 * time.Second
	maxHTMLForGenerate = 500 * 1024 // 500 KB
)

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

// ExtractTestResult is a single scored candidate returned by /extracttest.
type ExtractTestResult struct {
	ConfigName      string `json:"configName"`
	FieldsWithValue int    `json:"fieldsWithValue"`
}

// ---------------------------------------------------------------------------
// APIClient
// ---------------------------------------------------------------------------

// APIClient communicates with the monoes config API.
type APIClient struct {
	baseURL    string
	httpClient *http.Client
	logger     zerolog.Logger
}

// APIClientOption is a functional option for APIClient.
type APIClientOption func(*APIClient)

// WithBaseURL overrides the default API base URL.
func WithBaseURL(url string) APIClientOption {
	return func(c *APIClient) { c.baseURL = url }
}

// WithHTTPClient provides a custom *http.Client.
func WithHTTPClient(hc *http.Client) APIClientOption {
	return func(c *APIClient) { c.httpClient = hc }
}

// NewAPIClient creates an APIClient with sensible defaults.
func NewAPIClient(logger zerolog.Logger, opts ...APIClientOption) *APIClient {
	c := &APIClient{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{}, // No client-level timeout; per-request contexts control deadlines.
		logger:     logger,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// ExtractTest sends HTML content to POST /extracttest and returns scored
// config candidates.
func (c *APIClient) ExtractTest(ctx context.Context, configName, htmlContent string) ([]ExtractTestResult, error) {
	body := map[string]string{
		"configName":  configName,
		"htmlContent": htmlContent,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal extracttest body: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+"/extracttest", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create extracttest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug().Str("endpoint", "POST /extracttest").Str("configName", configName).Msg("API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extracttest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extracttest returned %d: %s", resp.StatusCode, string(respBody))
	}

	var results []ExtractTestResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("decode extracttest response: %w", err)
	}
	return results, nil
}

// GetConfig fetches a complete config by name via GET /configs/{name}.
func (c *APIClient) GetConfig(ctx context.Context, name string) (map[string]interface{}, error) {
	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.baseURL+"/configs/"+name, nil)
	if err != nil {
		return nil, fmt.Errorf("create get-config request: %w", err)
	}

	c.logger.Debug().Str("endpoint", "GET /configs/"+name).Msg("API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get-config request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get-config returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode get-config response: %w", err)
	}
	return result, nil
}

// GenerateConfig sends HTML + schema to POST /generate-config for LLM-based
// config generation. Uses a longer timeout than other endpoints.
func (c *APIClient) GenerateConfig(
	ctx context.Context,
	configName, htmlContent, purpose string,
	schema map[string]interface{},
) (map[string]interface{}, error) {
	// Truncate HTML to avoid overwhelming the LLM.
	if len(htmlContent) > maxHTMLForGenerate {
		htmlContent = htmlContent[:maxHTMLForGenerate]
	}

	body := map[string]interface{}{
		"configName":       configName,
		"htmlContent":      htmlContent,
		"purpose":          purpose,
		"extractionSchema": schema,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal generate-config body: %w", err)
	}

	// Use a longer timeout context for LLM-backed generation.
	genCtx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(genCtx, http.MethodPost, c.baseURL+"/generate-config", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create generate-config request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug().Str("endpoint", "POST /generate-config").Str("configName", configName).Msg("API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("generate-config request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("generate-config returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode generate-config response: %w", err)
	}
	return result, nil
}
