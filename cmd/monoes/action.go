package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/monoes/monoes-agent/internal/storage"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newActionCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "action",
		Short: "Manage actions",
		Long:  "Create, list, inspect, pause, resume, delete actions and their targets.",
	}

	cmd.AddCommand(
		newActionListCmd(cfg),
		newActionGetCmd(cfg),
		newActionCreateCmd(cfg),
		newActionImportCmd(cfg),
		newActionPauseCmd(cfg),
		newActionResumeCmd(cfg),
		newActionDeleteCmd(cfg),
		newActionTargetsCmd(cfg),
	)

	return cmd
}

func newActionListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all actions",
		Example: `  monoes action list
  monoes action list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			rows, err := db.DB.Query(
				`SELECT id, created_at, title, type, state, target_platform, created_at_ts
				 FROM actions ORDER BY position ASC, created_at DESC`,
			)
			if err != nil {
				return fmt.Errorf("querying actions: %w", err)
			}
			defer rows.Close()

			type actionRow struct {
				ID        string    `json:"id"`
				CreatedAt int64     `json:"created_at"`
				Title     string    `json:"title"`
				Type      string    `json:"type"`
				State     string    `json:"state"`
				Platform  string    `json:"target_platform"`
				CreatedTS time.Time `json:"created_at_ts"`
			}

			var actions []actionRow
			for rows.Next() {
				var a actionRow
				if err := rows.Scan(
					&a.ID, &a.CreatedAt, &a.Title, &a.Type, &a.State,
					&a.Platform, &a.CreatedTS,
				); err != nil {
					return fmt.Errorf("scanning action: %w", err)
				}
				actions = append(actions, a)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating actions: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(actions)
			}

			if len(actions) == 0 {
				fmt.Println("No actions found.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Type", "Platform", "State", "Created"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, a := range actions {
				shortID := a.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				table.Append([]string{
					shortID,
					a.Type,
					a.Platform,
					a.State,
					a.CreatedTS.Format("2006-01-02 15:04"),
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d action(s)\n", len(actions))
			return nil
		},
	}
}

func newActionGetCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show action details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			act, err := loadActionByID(db, actionID)
			if err != nil {
				return err
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(act)
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Field", "Value"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)
			table.SetColumnAlignment([]int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT})

			table.Append([]string{"ID", act.ID})
			table.Append([]string{"Title", act.Title})
			table.Append([]string{"Type", act.Type})
			table.Append([]string{"State", act.State})
			table.Append([]string{"Platform", act.TargetPlatform})
			table.Append([]string{"Position", fmt.Sprintf("%d", act.Position)})
			table.Append([]string{"Disabled", fmt.Sprintf("%v", act.Disabled)})
			table.Append([]string{"Reached Index", fmt.Sprintf("%d", act.ReachedIndex)})
			table.Append([]string{"Exec Count", fmt.Sprintf("%d", act.ActionExecutionCount)})

			if act.Keywords != "" {
				table.Append([]string{"Keywords", act.Keywords})
			}
			if act.ContentSubject != "" {
				table.Append([]string{"Subject", act.ContentSubject})
			}
			if act.ContentMessage != "" {
				table.Append([]string{"Message", truncateStr(act.ContentMessage, 60)})
			}
			if act.ScheduledDate != "" {
				table.Append([]string{"Scheduled", act.ScheduledDate})
			}
			if act.StartDate != "" {
				table.Append([]string{"Start Date", act.StartDate})
			}
			if act.EndDate != "" {
				table.Append([]string{"End Date", act.EndDate})
			}
			if act.CampaignID != "" {
				table.Append([]string{"Campaign ID", act.CampaignID})
			}

			table.Render()
			return nil
		},
	}
}

func newActionCreateCmd(cfg *globalConfig) *cobra.Command {
	var (
		actionType string
		platform   string
		keyword    string
		message    string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new action from flags",
		Example: `  monoes action create --type KEYWORD_SEARCH --platform instagram --keyword "golang"
  monoes action create --type send_dms --platform linkedin --message "Hello!"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if actionType == "" {
				return fmt.Errorf("--type is required")
			}
			if platform == "" {
				return fmt.Errorf("--platform is required")
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			act := &storage.Action{
				ID:             storage.NewID(),
				CreatedAt:      time.Now().Unix(),
				Title:          fmt.Sprintf("%s on %s", strings.ToUpper(actionType), strings.ToLower(platform)),
				Type:           strings.ToUpper(actionType),
				State:          "PENDING",
				TargetPlatform: strings.ToLower(platform),
				Keywords:       keyword,
				ContentMessage: message,
			}

			if err := upsertAction(db, act); err != nil {
				return fmt.Errorf("creating action: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(act)
			}

			fmt.Fprintf(os.Stdout, "Created action %s [%s] on %s\n", act.ID, act.Type, act.TargetPlatform)
			return nil
		},
	}

	cmd.Flags().StringVar(&actionType, "type", "", "Action type (required)")
	cmd.Flags().StringVar(&platform, "platform", "", "Target platform (required)")
	cmd.Flags().StringVar(&keyword, "keyword", "", "Search keyword")
	cmd.Flags().StringVar(&message, "message", "", "Message content")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("platform")

	return cmd
}

func newActionImportCmd(cfg *globalConfig) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import an action from a JSON file",
		Example: `  monoes action import --file action.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("reading file %s: %w", filePath, err)
			}

			act, err := storage.ParseAction(data)
			if err != nil {
				return fmt.Errorf("parsing action: %w", err)
			}

			if err := upsertAction(db, act); err != nil {
				return fmt.Errorf("saving action: %w", err)
			}

			// Check if the JSON also contains targets.
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err == nil {
				if targets, ok := raw["targets"]; ok {
					if targetList, ok := targets.([]interface{}); ok {
						for _, t := range targetList {
							tMap, ok := t.(map[string]interface{})
							if !ok {
								continue
							}
							targetID := storage.NewID()
							tPlatform := act.TargetPlatform
							if p, ok := tMap["platform"].(string); ok && p != "" {
								tPlatform = p
							}
							link, _ := tMap["link"].(string)
							sourceType, _ := tMap["source_type"].(string)
							personID, _ := tMap["person_id"].(string)

							_, insertErr := db.DB.Exec(
								`INSERT INTO action_targets (id, action_id, person_id, platform, link, source_type, status)
								 VALUES (?, ?, ?, ?, ?, ?, 'PENDING')`,
								targetID, act.ID, personID, tPlatform, link, sourceType,
							)
							if insertErr != nil {
								fmt.Fprintf(os.Stderr, "Warning: could not create target: %v\n", insertErr)
							}
						}
						fmt.Fprintf(os.Stderr, "Imported %d target(s) for action.\n", len(targetList))
					}
				}
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(act)
			}

			fmt.Fprintf(os.Stdout, "Imported action %s [%s] on %s (state: %s)\n",
				act.ID, act.Type, act.TargetPlatform, act.State)
			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to action JSON file (required)")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func newActionPauseCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause an action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Verify action exists.
			var currentState string
			err = db.DB.QueryRow("SELECT state FROM actions WHERE id = ?", actionID).Scan(&currentState)
			if err == sql.ErrNoRows {
				return fmt.Errorf("action %q not found", actionID)
			}
			if err != nil {
				return fmt.Errorf("checking action: %w", err)
			}

			_, err = db.DB.Exec(
				"UPDATE actions SET state = 'PAUSED', updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
				actionID,
			)
			if err != nil {
				return fmt.Errorf("pausing action: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Action %s paused (was %s).\n", actionID, currentState)
			return nil
		},
	}
}

func newActionResumeCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a paused action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Verify action exists.
			var currentState string
			err = db.DB.QueryRow("SELECT state FROM actions WHERE id = ?", actionID).Scan(&currentState)
			if err == sql.ErrNoRows {
				return fmt.Errorf("action %q not found", actionID)
			}
			if err != nil {
				return fmt.Errorf("checking action: %w", err)
			}

			_, err = db.DB.Exec(
				"UPDATE actions SET state = 'PENDING', updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
				actionID,
			)
			if err != nil {
				return fmt.Errorf("resuming action: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Action %s resumed (was %s).\n", actionID, currentState)
			return nil
		},
	}
}

func newActionDeleteCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an action and its targets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Verify action exists.
			var existingID string
			err = db.DB.QueryRow("SELECT id FROM actions WHERE id = ?", actionID).Scan(&existingID)
			if err == sql.ErrNoRows {
				return fmt.Errorf("action %q not found", actionID)
			}
			if err != nil {
				return fmt.Errorf("checking action: %w", err)
			}

			// Delete targets first.
			targetResult, _ := db.DB.Exec("DELETE FROM action_targets WHERE action_id = ?", actionID)
			targetCount, _ := targetResult.RowsAffected()

			result, err := db.DB.Exec("DELETE FROM actions WHERE id = ?", actionID)
			if err != nil {
				return fmt.Errorf("deleting action: %w", err)
			}

			affected, _ := result.RowsAffected()
			if affected == 0 {
				return fmt.Errorf("action %q not found", actionID)
			}

			fmt.Fprintf(os.Stdout, "Deleted action %s and %d target(s).\n", actionID, targetCount)
			return nil
		},
	}
}

func newActionTargetsCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "targets <id>",
		Short: "List targets for an action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			rows, err := db.DB.Query(
				`SELECT id, action_id, COALESCE(person_id,''), platform,
				        COALESCE(link,''), COALESCE(source_type,''), status,
				        COALESCE(last_interacted_at,''), COALESCE(comment_text,''),
				        COALESCE(metadata,''), created_at
				 FROM action_targets WHERE action_id = ? ORDER BY created_at ASC`, actionID,
			)
			if err != nil {
				return fmt.Errorf("querying targets: %w", err)
			}
			defer rows.Close()

			var targets []storage.ActionTarget
			for rows.Next() {
				var t storage.ActionTarget
				if err := rows.Scan(
					&t.ID, &t.ActionID, &t.PersonID, &t.Platform, &t.Link,
					&t.SourceType, &t.Status, &t.LastInteractedAt,
					&t.CommentText, &t.Metadata, &t.CreatedAt,
				); err != nil {
					return fmt.Errorf("scanning target: %w", err)
				}
				targets = append(targets, t)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating targets: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(targets)
			}

			if len(targets) == 0 {
				fmt.Println("No targets found for this action.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Platform", "Link", "Status", "Last Interacted"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, t := range targets {
				shortID := t.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				table.Append([]string{
					shortID,
					t.Platform,
					truncateStr(t.Link, 40),
					t.Status,
					t.LastInteractedAt,
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d target(s)\n", len(targets))
			return nil
		},
	}
}

// truncateStr shortens s to at most n characters, appending "..." if truncated.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
