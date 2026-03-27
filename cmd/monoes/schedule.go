package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newScheduleCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage action schedules",
		Long:  "List, add, and remove schedules for actions.",
	}

	cmd.AddCommand(
		newScheduleListCmd(cfg),
		newScheduleAddCmd(cfg),
		newScheduleRemoveCmd(cfg),
	)

	return cmd
}

func newScheduleListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scheduled actions",
		Example: `  monoes schedule list
  monoes schedule list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			rows, err := db.DB.Query(
				`SELECT id, title, type, state, target_platform,
				        COALESCE(scheduled_date,''), COALESCE(execution_interval,0),
				        COALESCE(start_date,''), COALESCE(end_date,'')
				 FROM actions
				 WHERE scheduled_date IS NOT NULL AND scheduled_date != ''
				 ORDER BY scheduled_date ASC`,
			)
			if err != nil {
				return fmt.Errorf("querying scheduled actions: %w", err)
			}
			defer rows.Close()

			type scheduledAction struct {
				ID                string `json:"id"`
				Title             string `json:"title"`
				Type              string `json:"type"`
				State             string `json:"state"`
				Platform          string `json:"target_platform"`
				ScheduledDate     string `json:"scheduled_date"`
				ExecutionInterval int    `json:"execution_interval"`
				StartDate         string `json:"start_date,omitempty"`
				EndDate           string `json:"end_date,omitempty"`
			}

			var scheduled []scheduledAction
			for rows.Next() {
				var s scheduledAction
				if err := rows.Scan(
					&s.ID, &s.Title, &s.Type, &s.State, &s.Platform,
					&s.ScheduledDate, &s.ExecutionInterval,
					&s.StartDate, &s.EndDate,
				); err != nil {
					return fmt.Errorf("scanning scheduled action: %w", err)
				}
				scheduled = append(scheduled, s)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating scheduled actions: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(scheduled)
			}

			if len(scheduled) == 0 {
				fmt.Println("No scheduled actions found.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Title", "Type", "Platform", "State", "Schedule", "Start", "End"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, s := range scheduled {
				shortID := s.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				table.Append([]string{
					shortID,
					truncateStr(s.Title, 20),
					s.Type,
					s.Platform,
					s.State,
					s.ScheduledDate,
					s.StartDate,
					s.EndDate,
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d scheduled action(s)\n", len(scheduled))
			return nil
		},
	}
}

func newScheduleAddCmd(cfg *globalConfig) *cobra.Command {
	var (
		cron      string
		startDate string
		endDate   string
	)

	cmd := &cobra.Command{
		Use:   "add <action-id>",
		Short: "Schedule an action",
		Example: `  monoes schedule add abc-123 --cron "0 9 * * *"
  monoes schedule add abc-123 --cron "0 9 * * *" --start-date 2025-01-01 --end-date 2025-12-31`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]

			if cron == "" {
				return fmt.Errorf("--cron is required")
			}

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

			// Build update query.
			setClauses := "scheduled_date = ?, updated_at_ts = CURRENT_TIMESTAMP"
			params := []interface{}{cron}

			if startDate != "" {
				if _, err := time.Parse("2006-01-02", startDate); err != nil {
					return fmt.Errorf("invalid --start-date format (expected YYYY-MM-DD): %w", err)
				}
				setClauses += ", start_date = ?"
				params = append(params, startDate)
			}

			if endDate != "" {
				if _, err := time.Parse("2006-01-02", endDate); err != nil {
					return fmt.Errorf("invalid --end-date format (expected YYYY-MM-DD): %w", err)
				}
				setClauses += ", end_date = ?"
				params = append(params, endDate)
			}

			params = append(params, actionID)

			_, err = db.DB.Exec(
				fmt.Sprintf("UPDATE actions SET %s WHERE id = ?", setClauses),
				params...,
			)
			if err != nil {
				return fmt.Errorf("scheduling action: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"action_id":  actionID,
					"cron":       cron,
					"start_date": startDate,
					"end_date":   endDate,
				})
			}

			fmt.Fprintf(os.Stdout, "Scheduled action %s with cron %q\n", actionID, cron)
			if startDate != "" {
				fmt.Fprintf(os.Stdout, "  Start date: %s\n", startDate)
			}
			if endDate != "" {
				fmt.Fprintf(os.Stdout, "  End date: %s\n", endDate)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cron, "cron", "", "Cron expression for scheduling (required)")
	cmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")
	_ = cmd.MarkFlagRequired("cron")

	return cmd
}

func newScheduleRemoveCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <action-id>",
		Short: "Remove schedule from an action",
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

			_, err = db.DB.Exec(
				`UPDATE actions SET scheduled_date = NULL, start_date = NULL, end_date = NULL,
				 execution_interval = NULL, updated_at_ts = CURRENT_TIMESTAMP
				 WHERE id = ?`,
				actionID,
			)
			if err != nil {
				return fmt.Errorf("removing schedule: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Removed schedule from action %s.\n", actionID)
			return nil
		},
	}
}
