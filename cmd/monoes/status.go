package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newStatusCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show system status overview",
		Long:  "Displays database path, session count, action counts by state, people count, and config count.",
		Example: `  monoes status
  monoes status --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			dbPath := expandPath(cfg.DBPath)

			// Session count.
			var sessionCount int
			err = db.DB.QueryRow("SELECT COUNT(*) FROM crawler_sessions").Scan(&sessionCount)
			if err != nil {
				sessionCount = 0
			}

			// People count.
			var peopleCount int
			err = db.DB.QueryRow("SELECT COUNT(*) FROM people").Scan(&peopleCount)
			if err != nil {
				peopleCount = 0
			}

			// Config count.
			var configCount int
			err = db.DB.QueryRow("SELECT COUNT(*) FROM configs").Scan(&configCount)
			if err != nil {
				configCount = 0
			}

			// Action counts by state.
			type stateCount struct {
				State string `json:"state"`
				Count int    `json:"count"`
			}
			var actionCounts []stateCount
			var totalActions int

			rows, err := db.DB.Query(
				"SELECT state, COUNT(*) FROM actions GROUP BY state ORDER BY state",
			)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var sc stateCount
					if scanErr := rows.Scan(&sc.State, &sc.Count); scanErr == nil {
						actionCounts = append(actionCounts, sc)
						totalActions += sc.Count
					}
				}
			}

			// Template count.
			var templateCount int
			err = db.DB.QueryRow("SELECT COUNT(*) FROM templates").Scan(&templateCount)
			if err != nil {
				templateCount = 0
			}

			// List count.
			var listCount int
			err = db.DB.QueryRow("SELECT COUNT(*) FROM social_lists").Scan(&listCount)
			if err != nil {
				listCount = 0
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"db_path":       dbPath,
					"sessions":      sessionCount,
					"people":        peopleCount,
					"total_actions": totalActions,
					"action_states": actionCounts,
					"configs":       configCount,
					"templates":     templateCount,
					"social_lists":  listCount,
				})
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Metric", "Value"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)
			table.SetColumnAlignment([]int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT})

			table.Append([]string{"Database Path", dbPath})
			table.Append([]string{"Sessions", fmt.Sprintf("%d", sessionCount)})
			table.Append([]string{"People", fmt.Sprintf("%d", peopleCount)})
			table.Append([]string{"Total Actions", fmt.Sprintf("%d", totalActions)})

			for _, sc := range actionCounts {
				table.Append([]string{fmt.Sprintf("  %s", sc.State), fmt.Sprintf("%d", sc.Count)})
			}

			table.Append([]string{"Configs", fmt.Sprintf("%d", configCount)})
			table.Append([]string{"Templates", fmt.Sprintf("%d", templateCount)})
			table.Append([]string{"Social Lists", fmt.Sprintf("%d", listCount)})
			table.Render()

			return nil
		},
	}
}
