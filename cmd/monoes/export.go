package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nokhodian/mono-agent/internal/storage"
	"github.com/spf13/cobra"
)

func newExportCmd(cfg *globalConfig) *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all data to JSON files",
		Long: `Exports all people and actions (with targets) to JSON files.
Files are written to the output directory, defaulting to the global --output-dir.`,
		Example: `  monoes export
  monoes export --output-dir ./backup`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			dir := outputDir
			if dir == "" {
				dir = cfg.OutputDir
			}
			dir = expandPath(dir)

			if err := ensureDir(dir); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			// Export people.
			peopleCount, err := exportPeopleData(db, dir)
			if err != nil {
				return fmt.Errorf("exporting people: %w", err)
			}

			// Export actions.
			actionsCount, err := exportActionsData(db, dir)
			if err != nil {
				return fmt.Errorf("exporting actions: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"output_dir":    dir,
					"people_count":  peopleCount,
					"actions_count": actionsCount,
				})
			}

			fmt.Fprintf(os.Stdout, "Exported %d people and %d actions to %s\n", peopleCount, actionsCount, dir)
			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory (defaults to global --output-dir)")

	return cmd
}

func exportPeopleData(db *storage.Database, outputDir string) (int, error) {
	rows, err := db.DB.Query(
		`SELECT id, platform_username, platform, COALESCE(full_name,''),
		        COALESCE(image_url,''), COALESCE(contact_details,''),
		        COALESCE(website,''), content_count, COALESCE(follower_count,''),
		        following_count, COALESCE(introduction,''), is_verified,
		        COALESCE(category,''), COALESCE(job_title,''),
		        created_at, updated_at
		 FROM people ORDER BY created_at DESC`,
	)
	if err != nil {
		return 0, fmt.Errorf("querying people: %w", err)
	}
	defer rows.Close()

	var people []storage.Person
	for rows.Next() {
		var p storage.Person
		var verified int
		if err := rows.Scan(
			&p.ID, &p.PlatformUsername, &p.Platform, &p.FullName,
			&p.ImageURL, &p.ContactDetails, &p.Website, &p.ContentCount,
			&p.FollowerCount, &p.FollowingCount, &p.Introduction,
			&verified, &p.Category, &p.JobTitle, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scanning person: %w", err)
		}
		p.IsVerified = verified != 0
		people = append(people, p)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating people: %w", err)
	}

	envelope := map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"count":       len(people),
		"people":      people,
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshaling people: %w", err)
	}

	path := filepath.Join(outputDir, "people_export.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 0, fmt.Errorf("writing people export: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Written %d people to %s\n", len(people), path)
	return len(people), nil
}

func exportActionsData(db *storage.Database, outputDir string) (int, error) {
	rows, err := db.DB.Query(
		`SELECT id, created_at, title, type, state, disabled, target_platform,
		        position, COALESCE(content_subject,''), COALESCE(content_message,''),
		        COALESCE(content_blob_urls,''), COALESCE(scheduled_date,''),
		        COALESCE(execution_interval,0), COALESCE(start_date,''),
		        COALESCE(end_date,''), COALESCE(campaign_id,''),
		        reached_index, COALESCE(keywords,''), action_execution_count
		 FROM actions ORDER BY position ASC, created_at DESC`,
	)
	if err != nil {
		return 0, fmt.Errorf("querying actions: %w", err)
	}
	defer rows.Close()

	type actionExport struct {
		storage.Action
		Targets []storage.ActionTarget `json:"targets"`
	}

	var actions []actionExport
	for rows.Next() {
		var a storage.Action
		var disabled int
		if err := rows.Scan(
			&a.ID, &a.CreatedAt, &a.Title, &a.Type, &a.State, &disabled,
			&a.TargetPlatform, &a.Position,
			&a.ContentSubject, &a.ContentMessage, &a.ContentBlobURLs,
			&a.ScheduledDate, &a.ExecutionInterval, &a.StartDate, &a.EndDate,
			&a.CampaignID, &a.ReachedIndex, &a.Keywords, &a.ActionExecutionCount,
		); err != nil {
			return 0, fmt.Errorf("scanning action: %w", err)
		}
		a.Disabled = disabled != 0
		actions = append(actions, actionExport{Action: a})
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating actions: %w", err)
	}

	// Load targets for each action.
	for i := range actions {
		tRows, err := db.DB.Query(
			`SELECT id, action_id, COALESCE(person_id,''), platform,
			        COALESCE(link,''), COALESCE(source_type,''), status,
			        COALESCE(last_interacted_at,''), COALESCE(comment_text,''),
			        COALESCE(metadata,''), created_at
			 FROM action_targets WHERE action_id = ? ORDER BY created_at ASC`,
			actions[i].ID,
		)
		if err != nil {
			return 0, fmt.Errorf("querying targets for action %s: %w", actions[i].ID, err)
		}

		for tRows.Next() {
			var t storage.ActionTarget
			if err := tRows.Scan(
				&t.ID, &t.ActionID, &t.PersonID, &t.Platform, &t.Link,
				&t.SourceType, &t.Status, &t.LastInteractedAt,
				&t.CommentText, &t.Metadata, &t.CreatedAt,
			); err != nil {
				tRows.Close()
				return 0, fmt.Errorf("scanning target: %w", err)
			}
			actions[i].Targets = append(actions[i].Targets, t)
		}
		tRows.Close()
	}

	envelope := map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"count":       len(actions),
		"actions":     actions,
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshaling actions: %w", err)
	}

	path := filepath.Join(outputDir, "actions_export.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 0, fmt.Errorf("writing actions export: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Written %d actions to %s\n", len(actions), path)
	return len(actions), nil
}
