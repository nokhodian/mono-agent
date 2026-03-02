package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FileStore provides JSON-file-based storage for extracted data and bulk
// exports alongside the primary SQLite database.
type FileStore struct {
	outputDir string
}

// NewFileStore creates a FileStore that writes files under outputDir.
func NewFileStore(outputDir string) *FileStore {
	return &FileStore{outputDir: outputDir}
}

// sanitize removes characters that are unsafe in filenames and collapses
// multiple underscores.
var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = unsafeChars.ReplaceAllString(s, "_")
	// Collapse multiple underscores.
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	if s == "" {
		s = "unnamed"
	}
	return s
}

// SaveExtractedData writes a slice of extracted items to a JSON file organized
// under outputDir/<platform>/<actionType>_<keyword>_<timestamp>.json.
//
// Parameters:
//   - actionID:   the action that produced this data (included in the file metadata)
//   - actionType: e.g. "KEYWORD_SEARCH", "PROFILE_FETCH" (used in the filename)
//   - platform:   e.g. "instagram", "linkedin" (used as a subdirectory)
//   - keyword:    optional search keyword (used in the filename when non-empty)
//   - items:      the data to serialize
func (fs *FileStore) SaveExtractedData(actionID, actionType, platform, keyword string, items []map[string]interface{}) error {
	safePlatform := sanitize(platform)
	dir := filepath.Join(fs.outputDir, safePlatform)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output directory %s: %w", dir, err)
	}

	// Build a descriptive filename.
	ts := time.Now().UTC().Format("20060102_150405")
	parts := []string{sanitize(actionType)}
	if keyword != "" {
		parts = append(parts, sanitize(keyword))
	}
	parts = append(parts, ts)
	filename := strings.Join(parts, "_") + ".json"
	filePath := filepath.Join(dir, filename)

	// Wrap the items in a metadata envelope.
	envelope := map[string]interface{}{
		"action_id":   actionID,
		"action_type": actionType,
		"platform":    platform,
		"keyword":     keyword,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"count":       len(items),
		"items":       items,
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling extracted data: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("writing extracted data to %s: %w", filePath, err)
	}

	return nil
}

// ExportAllData exports all people and actions from the database to JSON files
// in outputDir. This is useful for creating full backups or data exports.
func ExportAllData(db *Database, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating export directory %s: %w", outputDir, err)
	}

	// Export people.
	if err := exportPeople(db, outputDir); err != nil {
		return err
	}

	// Export actions.
	if err := exportActions(db, outputDir); err != nil {
		return err
	}

	return nil
}

// exportPeople writes all people records to a JSON file.
func exportPeople(db *Database, outputDir string) error {
	people, err := db.ListPeople("", "", 0, 0)
	if err != nil {
		return fmt.Errorf("loading people for export: %w", err)
	}

	// Fetch all — ListPeople with limit=0 defaults to 100, so we page through.
	var allPeople []*Person
	offset := 0
	batchSize := 500
	for {
		batch, err := db.ListPeople("", "", batchSize, offset)
		if err != nil {
			return fmt.Errorf("loading people batch at offset %d: %w", offset, err)
		}
		if len(batch) == 0 {
			break
		}
		allPeople = append(allPeople, batch...)
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	// If the first simple call got results and paging didn't, use the first call.
	if len(allPeople) == 0 {
		allPeople = people
	}

	envelope := map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"count":       len(allPeople),
		"people":      allPeople,
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling people export: %w", err)
	}

	path := filepath.Join(outputDir, "people_export.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing people export: %w", err)
	}

	return nil
}

// exportActions writes all actions and their targets to a JSON file.
func exportActions(db *Database, outputDir string) error {
	offset := 0
	batchSize := 500
	var allActions []map[string]interface{}

	for {
		batch, err := db.ListActions("", "", "", batchSize, offset)
		if err != nil {
			return fmt.Errorf("loading actions batch at offset %d: %w", offset, err)
		}
		if len(batch) == 0 {
			break
		}
		for _, action := range batch {
			// Load targets for each action.
			targets, err := db.ListActionTargets(action.ID, "")
			if err != nil {
				return fmt.Errorf("loading targets for action %s: %w", action.ID, err)
			}
			entry := map[string]interface{}{
				"action":  action,
				"targets": targets,
			}
			allActions = append(allActions, entry)
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	envelope := map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"count":       len(allActions),
		"actions":     allActions,
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling actions export: %w", err)
	}

	path := filepath.Join(outputDir, "actions_export.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing actions export: %w", err)
	}

	return nil
}
