//go:build integration

package config

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/rs/zerolog"
)

// Run with: go test -tags integration -run TestAPIClient ./internal/config/
// Requires the Python API to be running on localhost:8000.

func TestAPIClientIntegration(t *testing.T) {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

	client := NewAPIClient(logger, WithBaseURL("http://127.0.0.1:8000"))
	ctx := context.Background()

	sampleHTML := `<html><body><div id="loginForm">
		<input name="username" type="text" aria-label="Phone number, username, or email" />
		<input name="password" type="password" aria-label="Password" />
		<button type="submit" class="_acan">Log in</button>
	</div></body></html>`

	// --- Test 1: GenerateConfig ---
	t.Run("GenerateConfig", func(t *testing.T) {
		schema := GetSchema("instagram", "login", "")
		if schema == nil {
			t.Fatal("expected non-nil schema for instagram login")
		}

		result, err := client.GenerateConfig(ctx, "test_instagram_login", sampleHTML, "Find login selectors", schema)
		if err != nil {
			t.Fatalf("GenerateConfig failed: %v", err)
		}

		configName, ok := result["configName"].(string)
		if !ok || configName == "" {
			// Also check wrapped format
			if _, hasConfig := result["config"]; !hasConfig {
				t.Logf("result: %+v", result)
				t.Fatal("expected configName or config in response")
			}
		}
		t.Logf("GenerateConfig returned: configName=%v", result["configName"])
	})

	// --- Test 2: ExtractTest ---
	t.Run("ExtractTest", func(t *testing.T) {
		results, err := client.ExtractTest(ctx, "instagram_login", sampleHTML)
		if err != nil {
			t.Fatalf("ExtractTest failed: %v", err)
		}

		t.Logf("ExtractTest returned %d candidates", len(results))
		for _, r := range results {
			t.Logf("  %s: fieldsWithValue=%d", r.ConfigName, r.FieldsWithValue)
		}

		if len(results) == 0 {
			t.Fatal("expected at least 1 candidate from ExtractTest")
		}
	})

	// --- Test 3: GetConfig ---
	t.Run("GetConfig", func(t *testing.T) {
		// First get the list of configs to find a valid name.
		results, err := client.ExtractTest(ctx, "instagram_login", sampleHTML)
		if err != nil || len(results) == 0 {
			t.Skip("no configs available to fetch")
		}

		configName := results[0].ConfigName
		cfg, err := client.GetConfig(ctx, configName)
		if err != nil {
			t.Fatalf("GetConfig(%s) failed: %v", configName, err)
		}

		if cfg == nil {
			t.Fatal("expected non-nil config")
		}

		t.Logf("GetConfig returned config with keys: ")
		for k := range cfg {
			t.Logf("  %s", k)
		}
	})

	// --- Test 4: parseAPIConfig ---
	t.Run("ParseAPIConfig_Wrapped", func(t *testing.T) {
		raw := map[string]interface{}{
			"configName": "test_config.json",
			"config": map[string]interface{}{
				"config_name": "test_config",
				"fields": map[string]interface{}{
					"name":  "login_form",
					"xpath": "//div[@id='loginForm']",
					"type":  "array",
					"data":  []interface{}{},
				},
			},
		}

		cfg, err := parseAPIConfig(raw)
		if err != nil {
			t.Fatalf("parseAPIConfig (wrapped) failed: %v", err)
		}
		if cfg.ConfigName != "test_config" {
			t.Errorf("expected config name 'test_config', got %q", cfg.ConfigName)
		}
		t.Logf("Parsed wrapped config: %s", cfg.ConfigName)
	})

	t.Run("ParseAPIConfig_Direct", func(t *testing.T) {
		raw := map[string]interface{}{
			"config_name": "direct_config",
			"fields": map[string]interface{}{
				"name":  "login_form",
				"xpath": "//div[@id='loginForm']",
				"type":  "array",
				"data":  []interface{}{},
			},
		}

		cfg, err := parseAPIConfig(raw)
		if err != nil {
			t.Fatalf("parseAPIConfig (direct) failed: %v", err)
		}
		if cfg.ConfigName != "direct_config" {
			t.Errorf("expected config name 'direct_config', got %q", cfg.ConfigName)
		}
		t.Logf("Parsed direct config: %s", cfg.ConfigName)
	})

	t.Run("ParseAPIConfig_BareFields", func(t *testing.T) {
		// Simulates GET /configs/{name} response — bare fields object.
		raw := map[string]interface{}{
			"name":  "login_form",
			"xpath": "//div[@id='loginForm']",
			"type":  "array",
			"data":  []interface{}{},
		}

		cfg, err := parseAPIConfig(raw)
		if err != nil {
			t.Fatalf("parseAPIConfig (bare fields) failed: %v", err)
		}
		// ConfigName will be empty for bare fields (caller sets it).
		if cfg.Fields.Name != "login_form" {
			t.Errorf("expected fields.name 'login_form', got %q", cfg.Fields.Name)
		}
		t.Logf("Parsed bare fields: fields.name=%s", cfg.Fields.Name)
	})

	// --- Test: Full ConfigManager 3-tier flow ---
	t.Run("ConfigManager_FullFlow", func(t *testing.T) {
		// Use an in-memory ConfigStore that does nothing.
		store := &memConfigStore{configs: make(map[string]*Config)}
		mgr := NewConfigManager(t.TempDir(), store, client, logger)

		// Tier 2 should hit the API (testViaAPI) since there are no local files.
		cfg, err := mgr.GetConfig("instagram", "login", "", sampleHTML, "Find login selectors", nil)
		if err != nil {
			t.Fatalf("ConfigManager.GetConfig failed: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected non-nil config from ConfigManager")
		}
		t.Logf("ConfigManager resolved: %s (fields.name=%s, fields.xpath=%s)",
			cfg.ConfigName, cfg.Fields.Name, cfg.Fields.XPath)

		// Verify the config was cached.
		cfg2, err := mgr.GetConfig("instagram", "login", "", "", "", nil)
		if err != nil {
			t.Fatalf("Second GetConfig (cached) failed: %v", err)
		}
		t.Logf("Cached lookup: %s", cfg2.ConfigName)
	})
}

// memConfigStore is a simple in-memory ConfigStore for testing.
type memConfigStore struct {
	configs map[string]*Config
}

func (m *memConfigStore) GetConfig(name string) (*Config, error) {
	cfg, ok := m.configs[name]
	if !ok {
		return nil, nil
	}
	return cfg, nil
}

func (m *memConfigStore) SaveConfig(name string, configData string) error {
	var cfg Config
	if err := json.Unmarshal([]byte(configData), &cfg); err != nil {
		return err
	}
	m.configs[name] = &cfg
	return nil
}
