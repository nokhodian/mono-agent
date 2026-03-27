package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/monoes/monoes-agent/internal/storage"
	"github.com/spf13/cobra"
)

func newSearchCmd(cfg *globalConfig) *cobra.Command {
	var (
		keyword string
		maxResults int
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "search <platform>",
		Short: "Quick keyword search on a platform",
		Long:  "Creates a KEYWORD_SEARCH action and immediately executes it. Results are saved to the database.",
		Example: `  monoes search instagram --keyword "golang developer" --max 50
  monoes search linkedin --keyword "startup founder" --max 100
  monoes search x --keyword "AI engineer"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyword == "" {
				return fmt.Errorf("--keyword is required")
			}

			platform := strings.ToLower(args[0])

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Create a KEYWORD_SEARCH action.
			actionID := storage.NewID()
			act := &storage.Action{
				ID:             actionID,
				CreatedAt:      time.Now().Unix(),
				Title:          fmt.Sprintf("Quick search: %s on %s", keyword, platform),
				Type:           "KEYWORD_SEARCH",
				State:          "PENDING",
				TargetPlatform: platform,
				Keywords:       keyword,
			}

			if err := upsertAction(db, act); err != nil {
				return fmt.Errorf("creating search action: %w", err)
			}

			// Create a single target representing the search.
			targetID := storage.NewID()
			_, err = db.DB.Exec(
				`INSERT INTO action_targets (id, action_id, platform, status, metadata)
				 VALUES (?, ?, ?, 'PENDING', ?)`,
				targetID, actionID, platform,
				fmt.Sprintf(`{"keyword":"%s","max_results":%d}`, keyword, maxResults),
			)
			if err != nil {
				return fmt.Errorf("creating search target: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Created search action %s for keyword %q on %s (max: %d)\n",
				actionID, keyword, platform, maxResults)

			// Execute immediately.
			ctx := cmd.Context()
			return executeAction(ctx, db, cfg, act, timeout)
		},
	}

	cmd.Flags().StringVar(&keyword, "keyword", "", "Search keyword (required)")
	cmd.Flags().IntVar(&maxResults, "max", 50, "Maximum number of results to collect")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum execution time")
	_ = cmd.MarkFlagRequired("keyword")

	return cmd
}
