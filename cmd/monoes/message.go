package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/monoes/monoes-agent/internal/storage"
	"github.com/spf13/cobra"
)

func newMessageCmd(cfg *globalConfig) *cobra.Command {
	var (
		text    string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "message <platform> <username>",
		Short: "Send a quick direct message",
		Long:  "Creates a send_dms action with a single target and executes it immediately.",
		Example: `  monoes message instagram johndoe --text "hello"
  monoes message linkedin janedoe --text "Hi, let's connect!"
  monoes message x devuser --text "Great post!" --timeout 5m`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if text == "" {
				return fmt.Errorf("--text is required")
			}

			platform := strings.ToLower(args[0])
			username := args[1]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Create a send_dms action with one target.
			actionID := storage.NewID()
			act := &storage.Action{
				ID:             actionID,
				CreatedAt:      time.Now().Unix(),
				Title:          fmt.Sprintf("Quick DM to %s on %s", username, platform),
				Type:           "send_dms",
				State:          "PENDING",
				TargetPlatform: platform,
				ContentMessage: text,
			}

			if err := upsertAction(db, act); err != nil {
				return fmt.Errorf("creating message action: %w", err)
			}

			// Create a single target for this user.
			targetID := storage.NewID()
			_, err = db.DB.Exec(
				`INSERT INTO action_targets (id, action_id, platform, link, status, metadata)
				 VALUES (?, ?, ?, ?, 'PENDING', ?)`,
				targetID, actionID, platform, username,
				fmt.Sprintf(`{"username":"%s"}`, username),
			)
			if err != nil {
				return fmt.Errorf("creating target for %s: %w", username, err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"action_id": actionID,
					"platform":  platform,
					"username":  username,
					"message":   text,
				})
			}

			fmt.Fprintf(os.Stderr, "Created messaging action %s for %s on %s\n", actionID, username, platform)

			// Execute immediately.
			ctx := cmd.Context()
			return executeAction(ctx, db, cfg, act, timeout)
		},
	}

	cmd.Flags().StringVar(&text, "text", "", "Message text to send (required)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum execution time")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}
