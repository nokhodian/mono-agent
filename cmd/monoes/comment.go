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

func newCommentCmd(cfg *globalConfig) *cobra.Command {
	var (
		text    string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "comment <platform> <post-url> [post-url...]",
		Short: "Comment on one or more posts",
		Long:  "Creates a comment_on_posts action and executes it immediately. Supports multiple post URLs.",
		Example: `  monoes comment instagram https://www.instagram.com/p/ABC123/ --text "Great post!"
  monoes comment instagram https://www.instagram.com/p/ABC123/ https://www.instagram.com/p/DEF456/ --text "Nice!"
  monoes comment instagram --text "Love this!" https://www.instagram.com/p/ABC123/`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if text == "" {
				return fmt.Errorf("--text is required")
			}

			platform := strings.ToLower(args[0])
			postURLs := args[1:]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Create a comment_on_posts action.
			actionID := storage.NewID()
			act := &storage.Action{
				ID:             actionID,
				CreatedAt:      time.Now().Unix(),
				Title:          fmt.Sprintf("Comment on %d post(s) on %s", len(postURLs), platform),
				Type:           "comment_on_posts",
				State:          "PENDING",
				TargetPlatform: platform,
				ContentMessage: text,
			}

			if err := upsertAction(db, act); err != nil {
				return fmt.Errorf("creating comment action: %w", err)
			}

			// Create a target for each post URL.
			for _, postURL := range postURLs {
				postURL = strings.TrimSpace(postURL)
				if postURL == "" {
					continue
				}
				targetID := storage.NewID()
				_, err = db.DB.Exec(
					`INSERT INTO action_targets (id, action_id, platform, link, status, metadata)
					 VALUES (?, ?, ?, ?, 'PENDING', ?)`,
					targetID, actionID, platform, postURL,
					fmt.Sprintf(`{"url":"%s"}`, postURL),
				)
				if err != nil {
					return fmt.Errorf("creating target for %s: %w", postURL, err)
				}
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"action_id": actionID,
					"platform":  platform,
					"posts":     postURLs,
					"comment":   text,
				})
			}

			fmt.Fprintf(os.Stderr, "Created comment action %s for %d post(s) on %s\n",
				actionID, len(postURLs), platform)

			// Execute immediately.
			ctx := cmd.Context()
			return executeAction(ctx, db, cfg, act, timeout)
		},
	}

	cmd.Flags().StringVar(&text, "text", "", "Comment text to post (required)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum execution time")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}
