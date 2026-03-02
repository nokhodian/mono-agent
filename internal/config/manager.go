package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

// ErrConfigResolutionFailed is returned when all three tiers of config
// resolution fail.
type ErrConfigResolutionFailed struct {
	BaseName   string
	Tier1Error error // cache / active config lookup
	Tier2Error error // local file / database test
	Tier3Error error // config generation
}

func (e *ErrConfigResolutionFailed) Error() string {
	return fmt.Sprintf(
		"config resolution failed for %q: tier1=%v, tier2=%v, tier3=%v",
		e.BaseName, e.Tier1Error, e.Tier2Error, e.Tier3Error,
	)
}

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

// ConfigField represents a node in the configuration tree. Leaf nodes carry
// an XPath; branch nodes carry child fields in Data.
type ConfigField struct {
	Name  string        `json:"name"`
	XPath string        `json:"xpath,omitempty"`
	Data  []ConfigField `json:"data,omitempty"`
}

// Config is a named configuration consisting of a tree of ConfigField nodes.
type Config struct {
	ConfigName string      `json:"config_name"`
	Fields     ConfigField `json:"fields"`
}

// ---------------------------------------------------------------------------
// ConfigStore interface
// ---------------------------------------------------------------------------

// ConfigStore abstracts the persistence layer used by ConfigManager to load
// and save configuration data.
type ConfigStore interface {
	GetConfig(name string) (*Config, error)
	SaveConfig(name string, configData string) error
}

// ---------------------------------------------------------------------------
// ConfigManager
// ---------------------------------------------------------------------------

// ConfigManager implements the 3-tier config resolution strategy:
//
//  1. Cache — look up an already-resolved config from the in-memory map.
//  2. Test local — iterate over local config files (and optionally DB) and
//     validate them against the provided HTML content.
//  3. Generate — fall back to generating a new config (placeholder for future
//     LLM-based generation).
type ConfigManager struct {
	activeConfigs map[string]string // baseName -> fullConfigName
	mu            sync.RWMutex
	forceRefresh  bool
	configDir     string
	db            ConfigStore
	api           *APIClient
	logger        zerolog.Logger
}

// NewConfigManager creates a ConfigManager that looks for local config files
// in configDir, persists configs via db, and calls the external API via api.
func NewConfigManager(configDir string, db ConfigStore, api *APIClient, logger zerolog.Logger) *ConfigManager {
	return &ConfigManager{
		activeConfigs: make(map[string]string),
		configDir:     configDir,
		db:            db,
		api:           api,
		logger:        logger,
	}
}

// SetForceRefresh enables or disables forced cache bypass on the next
// GetConfig call.
func (cm *ConfigManager) SetForceRefresh(force bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.forceRefresh = force
}

// GetConfig resolves a configuration for the given context using the 3-tier
// strategy.
//
// Parameters:
//   - social:      the social platform name (e.g. "linkedin").
//   - action:      the action being performed (e.g. "login").
//   - configContext: additional context string appended to the base name.
//   - htmlContent: the current page HTML used to validate candidate configs.
//   - purpose:     human-readable description used during generation (tier 3).
//   - schema:      JSON-schema-like map describing the desired config shape.
func (cm *ConfigManager) GetConfig(
	social, action, configContext, htmlContent, purpose string,
	schema map[string]interface{},
) (*Config, error) {
	baseName := social + "_" + action
	if configContext != "" {
		baseName = baseName + "_" + configContext
	}

	// ---- Tier 1: cache lookup ----
	cm.mu.RLock()
	forceRefresh := cm.forceRefresh
	cm.mu.RUnlock()

	if !forceRefresh {
		cm.mu.RLock()
		fullName, ok := cm.activeConfigs[baseName]
		cm.mu.RUnlock()

		if ok {
			cfg, err := cm.loadFromFile(fullName)
			if err == nil {
				cm.logger.Debug().
					Str("baseName", baseName).
					Str("fullName", fullName).
					Msg("config resolved from cache (tier 1, file)")
				return cfg, nil
			}
			// File not found — try the database.
			cfg, err = cm.loadFromDB(fullName)
			if err == nil && cfg != nil {
				cm.logger.Debug().
					Str("baseName", baseName).
					Str("fullName", fullName).
					Msg("config resolved from cache (tier 1, db)")
				return cfg, nil
			}
			// Cache entry is stale; fall through.
			cm.logger.Warn().
				Str("baseName", baseName).
				Msg("cached config could not be loaded, falling through to tier 2")
		}
	}

	tier1Err := fmt.Errorf("no cached config for %q (forceRefresh=%v)", baseName, forceRefresh)

	// ---- Tier 2: test local configs ----
	cfg, tier2Err := cm.testLocalConfigs(baseName, htmlContent)
	if tier2Err == nil {
		cm.logger.Debug().
			Str("baseName", baseName).
			Msg("config resolved from local/db (tier 2)")
		return cfg, nil
	}

	cm.logger.Warn().
		Err(tier2Err).
		Str("baseName", baseName).
		Msg("local config test failed, falling through to tier 3")

	// ---- Tier 3: generate ----
	cfg, tier3Err := cm.generateConfig(baseName, htmlContent, purpose, schema)
	if tier3Err == nil {
		cm.logger.Info().
			Str("baseName", baseName).
			Msg("config generated (tier 3)")
		return cfg, nil
	}

	return nil, &ErrConfigResolutionFailed{
		BaseName:   baseName,
		Tier1Error: tier1Err,
		Tier2Error: tier2Err,
		Tier3Error: tier3Err,
	}
}

// cacheConfig stores a resolved config in the in-memory active map.
func (cm *ConfigManager) cacheConfig(baseName string, config *Config) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.activeConfigs[baseName] = config.ConfigName
	cm.forceRefresh = false
	cm.logger.Debug().
		Str("baseName", baseName).
		Str("configName", config.ConfigName).
		Msg("config cached")
}

// loadFromFile reads a config JSON file from configDir.
func (cm *ConfigManager) loadFromFile(fullName string) (*Config, error) {
	path := filepath.Join(cm.configDir, fullName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config file %s: %w", path, err)
	}
	return &cfg, nil
}

// loadFromDB loads a config from the database via the ConfigStore interface.
func (cm *ConfigManager) loadFromDB(fullName string) (*Config, error) {
	if cm.db == nil {
		return nil, fmt.Errorf("no database configured")
	}
	return cm.db.GetConfig(fullName)
}

// testLocalConfigs iterates over candidate config files (and DB entries) that
// match baseName, attempting to find one that is valid for the current
// htmlContent.
func (cm *ConfigManager) testLocalConfigs(baseName, htmlContent string) (*Config, error) {
	// Try loading directly by baseName from file system.
	cfg, err := cm.loadFromFile(baseName)
	if err == nil {
		cm.cacheConfig(baseName, cfg)
		return cfg, nil
	}
	fileErr := err

	// Scan configDir for files that start with baseName (e.g. baseName_v2).
	pattern := filepath.Join(cm.configDir, baseName+"*.json")
	matches, globErr := filepath.Glob(pattern)
	if globErr == nil {
		for _, match := range matches {
			name := strings.TrimSuffix(filepath.Base(match), ".json")
			if name == baseName {
				continue // already tried above
			}
			candidate, loadErr := cm.loadFromFile(name)
			if loadErr != nil {
				cm.logger.Debug().Err(loadErr).Str("candidate", name).Msg("skipping candidate config")
				continue
			}
			// Accept the first loadable candidate.
			cm.cacheConfig(baseName, candidate)
			return candidate, nil
		}
	}

	// Try the database.
	cfg, dbErr := cm.loadFromDB(baseName)
	if dbErr == nil && cfg != nil {
		cm.cacheConfig(baseName, cfg)
		return cfg, nil
	}

	// Try the external API's /extracttest endpoint.
	if cm.api != nil && htmlContent != "" {
		apiCfg, apiErr := cm.testViaAPI(baseName, htmlContent)
		if apiErr == nil {
			return apiCfg, nil
		}
		cm.logger.Debug().Err(apiErr).Str("baseName", baseName).Msg("API extracttest failed")
	}

	return nil, fmt.Errorf(
		"no valid local config for %q (file: %v, db: %v)",
		baseName, fileErr, dbErr,
	)
}

// generateConfig calls the external API to generate a new config via LLM.
func (cm *ConfigManager) generateConfig(
	baseName, htmlContent, purpose string,
	schema map[string]interface{},
) (*Config, error) {
	if cm.api == nil {
		return nil, fmt.Errorf("no API client configured for generation of %q", baseName)
	}
	if htmlContent == "" {
		return nil, fmt.Errorf("no HTML content for config generation of %q", baseName)
	}

	if schema == nil {
		schema = map[string]interface{}{}
	}

	ctx := context.Background()
	raw, err := cm.api.GenerateConfig(ctx, baseName, htmlContent, purpose, schema)
	if err != nil {
		return nil, fmt.Errorf("generate config %q via API: %w", baseName, err)
	}

	cfg, err := parseAPIConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("parse generated config %q: %w", baseName, err)
	}

	// Normalize: strip .json suffix to avoid double-extension in loadFromFile.
	cfg.ConfigName = strings.TrimSuffix(cfg.ConfigName, ".json")

	// Persist to DB and cache for future lookups.
	cm.persistAndCache(baseName, cfg)
	return cfg, nil
}

// ---------------------------------------------------------------------------
// API-based config testing (Tier 2 extension)
// ---------------------------------------------------------------------------

// testViaAPI sends HTML to the API's /extracttest endpoint, picks the best
// scored candidate, fetches it, and persists it locally.
func (cm *ConfigManager) testViaAPI(baseName, htmlContent string) (*Config, error) {
	ctx := context.Background()

	results, err := cm.api.ExtractTest(ctx, baseName, htmlContent)
	if err != nil {
		return nil, fmt.Errorf("extracttest for %q: %w", baseName, err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("extracttest returned no candidates for %q", baseName)
	}

	// Sort by fieldsWithValue descending — best candidate first.
	sort.Slice(results, func(i, j int) bool {
		return results[i].FieldsWithValue > results[j].FieldsWithValue
	})

	best := results[0]
	if best.FieldsWithValue == 0 {
		return nil, fmt.Errorf("best candidate %q has 0 fields with value", best.ConfigName)
	}

	cm.logger.Debug().
		Str("baseName", baseName).
		Str("bestConfig", best.ConfigName).
		Int("fieldsWithValue", best.FieldsWithValue).
		Msg("API extracttest best candidate")

	// Fetch the full config from the API.
	raw, err := cm.api.GetConfig(ctx, best.ConfigName)
	if err != nil {
		return nil, fmt.Errorf("fetch best config %q: %w", best.ConfigName, err)
	}

	cfg, err := parseAPIConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("parse API config %q: %w", best.ConfigName, err)
	}

	// The GET /configs/{name} endpoint may return bare fields without a
	// config_name. Use the filename we already know.
	if cfg.ConfigName == "" {
		cfg.ConfigName = best.ConfigName
	}
	// Normalize: strip .json suffix to avoid double-extension in loadFromFile.
	cfg.ConfigName = strings.TrimSuffix(cfg.ConfigName, ".json")

	// Persist to DB and cache for future Tier 2 local lookups.
	cm.persistAndCache(baseName, cfg)
	return cfg, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// persistAndCache saves a config to the DB (if available) and caches it.
func (cm *ConfigManager) persistAndCache(baseName string, cfg *Config) {
	if cm.db != nil {
		data, err := json.Marshal(cfg)
		if err == nil {
			if saveErr := cm.db.SaveConfig(cfg.ConfigName, string(data)); saveErr != nil {
				cm.logger.Warn().Err(saveErr).Str("config", cfg.ConfigName).Msg("failed to persist config to DB")
			}
		}
	}
	cm.cacheConfig(baseName, cfg)
}

// parseAPIConfig converts a raw API response map into a *Config.
// It handles three response formats:
//   - Wrapped:    {"configName": "...", "config": {"name": "...", ...}}
//   - Direct:     {"config_name": "...", "fields": {...}}
//   - Bare fields: {"name": "login_form", "xpath": "...", "type": "array", "data": [...]}
//
// For bare fields (returned by GET /configs/{name}), the caller should set
// ConfigName on the returned *Config if it's empty.
func parseAPIConfig(raw map[string]interface{}) (*Config, error) {
	// Try wrapped format first: {"config": {...}, "configName": "..."}
	if inner, ok := raw["config"].(map[string]interface{}); ok {
		data, err := json.Marshal(inner)
		if err != nil {
			return nil, fmt.Errorf("marshal inner config: %w", err)
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal inner config: %w", err)
		}
		// If config_name is empty, try the outer configName.
		if cfg.ConfigName == "" {
			if name, ok := raw["configName"].(string); ok {
				cfg.ConfigName = name
			} else if name, ok := raw["config_name"].(string); ok {
				cfg.ConfigName = name
			}
		}
		return &cfg, nil
	}

	// Try direct format: {"config_name": "...", "fields": {...}}
	if _, hasFields := raw["fields"]; hasFields {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal raw config: %w", err)
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal raw config: %w", err)
		}
		if cfg.ConfigName == "" {
			if name, ok := raw["configName"].(string); ok {
				cfg.ConfigName = name
			}
		}
		return &cfg, nil
	}

	// Bare fields format: the response IS the fields object itself
	// (e.g. {"name": "login_form", "xpath": "...", "type": "array", "data": [...]})
	if _, hasName := raw["name"]; hasName {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal bare fields: %w", err)
		}
		var fields ConfigField
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("unmarshal bare fields: %w", err)
		}
		return &Config{
			Fields: fields,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized config format (keys: %v)", mapKeys(raw))
}

// mapKeys returns the keys of a map for diagnostic messages.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// ConfigManagerAdapter — bridges ConfigManager to action.ConfigInterface
// ---------------------------------------------------------------------------

// ConfigManagerAdapter wraps ConfigManager to satisfy the action package's
// ConfigInterface (which returns interface{} rather than *Config).
type ConfigManagerAdapter struct {
	Mgr *ConfigManager
}

// GetConfig resolves a config and returns the specific field's XPath string
// that resolveConfigSelector expects.
func (a *ConfigManagerAdapter) GetConfig(
	social, action, configContext, htmlContent, purpose string,
	schema map[string]interface{},
) (interface{}, error) {
	// Auto-resolve schema if the caller didn't provide one.
	if schema == nil {
		schema = GetSchema(social, action, configContext)
		if schema == nil {
			schema = map[string]interface{}{}
		}
	}

	cfg, err := a.Mgr.GetConfig(social, action, configContext, htmlContent, purpose, schema)
	if err != nil {
		return nil, err
	}

	// Use ConfigHelper to extract the XPath for the requested action field.
	helper := &ConfigHelper{}
	xpath, err := helper.GetXPath(cfg, action)
	if err != nil {
		// If the exact action field isn't found, return the whole config as a
		// map so the caller can inspect it.
		a.Mgr.logger.Debug().Err(err).Msg("GetXPath failed, returning config as map")
		data, _ := json.Marshal(cfg)
		var m map[string]interface{}
		json.Unmarshal(data, &m)
		return m, nil
	}

	return xpath, nil
}
