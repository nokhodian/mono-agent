package config

import (
	"encoding/json"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/storage"
)

// DBConfigStore adapts *storage.Database to satisfy the config.ConfigStore
// interface. It bridges the storage layer's ConfigEntry (raw JSON string) into
// the config package's *Config type.
type DBConfigStore struct {
	DB *storage.Database
}

// GetConfig retrieves a config by name from the database, parsing the stored
// JSON into a *Config. Returns (nil, nil) when not found.
func (s *DBConfigStore) GetConfig(name string) (*Config, error) {
	entry, err := s.DB.GetConfig(name)
	if err != nil {
		return nil, fmt.Errorf("db get config %s: %w", name, err)
	}
	if entry == nil {
		return nil, nil
	}

	var cfg Config
	if err := json.Unmarshal([]byte(entry.ConfigData), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config %s from db: %w", name, err)
	}
	return &cfg, nil
}

// SaveConfig persists a config to the database.
func (s *DBConfigStore) SaveConfig(name string, configData string) error {
	return s.DB.SaveConfig(name, configData)
}
