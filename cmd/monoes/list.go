package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nokhodian/mono-agent/internal/storage"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Manage social lists",
		Long:  "Create, view, and manage social lists and their items.",
	}

	cmd.AddCommand(
		newListLsCmd(cfg),
		newListCreateCmd(cfg),
		newListShowCmd(cfg),
		newListDeleteCmd(cfg),
		newListAddItemCmd(cfg),
	)

	return cmd
}

func newListLsCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "Show all social lists",
		Example: `  monoes list ls
  monoes list ls --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			rows, err := db.DB.Query(
				`SELECT id, COALESCE(list_type,''), name, item_count, created_at, updated_at
				 FROM social_lists ORDER BY created_at DESC`,
			)
			if err != nil {
				return fmt.Errorf("querying social lists: %w", err)
			}
			defer rows.Close()

			var lists []storage.SocialList
			for rows.Next() {
				var l storage.SocialList
				if err := rows.Scan(&l.ID, &l.ListType, &l.Name, &l.ItemCount, &l.CreatedAt, &l.UpdatedAt); err != nil {
					return fmt.Errorf("scanning social list: %w", err)
				}
				lists = append(lists, l)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating social lists: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(lists)
			}

			if len(lists) == 0 {
				fmt.Println("No social lists found.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Name", "Type", "Items", "Created"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, l := range lists {
				shortID := l.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				table.Append([]string{
					shortID,
					truncateStr(l.Name, 30),
					l.ListType,
					fmt.Sprintf("%d", l.ItemCount),
					l.CreatedAt.Format("2006-01-02 15:04"),
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d list(s)\n", len(lists))
			return nil
		},
	}
}

func newListCreateCmd(cfg *globalConfig) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new social list",
		Example: `  monoes list create --name "My List"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			listID := storage.NewID()
			now := time.Now().UTC()

			_, err = db.DB.Exec(
				`INSERT INTO social_lists (id, name, item_count, created_at, updated_at)
				 VALUES (?, ?, 0, ?, ?)`,
				listID, name, now, now,
			)
			if err != nil {
				return fmt.Errorf("creating list: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"id":   listID,
					"name": name,
				})
			}

			fmt.Fprintf(os.Stdout, "Created list %s (%s)\n", listID, name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "List name (required)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newListShowCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show items in a social list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			listID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Verify list exists.
			var listName string
			var itemCount int
			err = db.DB.QueryRow(
				"SELECT name, item_count FROM social_lists WHERE id = ?", listID,
			).Scan(&listName, &itemCount)
			if err == sql.ErrNoRows {
				return fmt.Errorf("list %q not found", listID)
			}
			if err != nil {
				return fmt.Errorf("querying list: %w", err)
			}

			rows, err := db.DB.Query(
				`SELECT id, list_id, platform, platform_username, COALESCE(image_url,''),
				        COALESCE(url,''), COALESCE(full_name,''), COALESCE(contact_details,''),
				        follower_count, created_at
				 FROM social_list_items WHERE list_id = ? ORDER BY created_at ASC`, listID,
			)
			if err != nil {
				return fmt.Errorf("querying list items: %w", err)
			}
			defer rows.Close()

			var items []storage.SocialListItem
			for rows.Next() {
				var item storage.SocialListItem
				if err := rows.Scan(
					&item.ID, &item.ListID, &item.Platform, &item.PlatformUsername,
					&item.ImageURL, &item.URL, &item.FullName, &item.ContactDetails,
					&item.FollowerCount, &item.CreatedAt,
				); err != nil {
					return fmt.Errorf("scanning list item: %w", err)
				}
				items = append(items, item)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating list items: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"list_id":    listID,
					"list_name":  listName,
					"item_count": itemCount,
					"items":      items,
				})
			}

			fmt.Fprintf(os.Stdout, "List: %s (%s)\n\n", listName, listID)

			if len(items) == 0 {
				fmt.Println("No items in this list.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Username", "Platform", "Name", "Followers"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, item := range items {
				shortID := item.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				table.Append([]string{
					shortID,
					truncateStr(item.PlatformUsername, 20),
					item.Platform,
					truncateStr(item.FullName, 20),
					fmt.Sprintf("%d", item.FollowerCount),
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d item(s)\n", len(items))
			return nil
		},
	}
}

func newListDeleteCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a social list and its items",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			listID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Delete items first.
			itemResult, _ := db.DB.Exec("DELETE FROM social_list_items WHERE list_id = ?", listID)
			itemCount, _ := itemResult.RowsAffected()

			result, err := db.DB.Exec("DELETE FROM social_lists WHERE id = ?", listID)
			if err != nil {
				return fmt.Errorf("deleting list: %w", err)
			}

			affected, _ := result.RowsAffected()
			if affected == 0 {
				return fmt.Errorf("list %q not found", listID)
			}

			fmt.Fprintf(os.Stdout, "Deleted list %s and %d item(s).\n", listID, itemCount)
			return nil
		},
	}
}

func newListAddItemCmd(cfg *globalConfig) *cobra.Command {
	var (
		username string
		platform string
	)

	cmd := &cobra.Command{
		Use:   "add-item <list-id>",
		Short: "Add an item to a social list",
		Example: `  monoes list add-item abc-123 --username johndoe --platform instagram`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			listID := args[0]

			if username == "" {
				return fmt.Errorf("--username is required")
			}
			if platform == "" {
				return fmt.Errorf("--platform is required")
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Verify list exists.
			var listName string
			err = db.DB.QueryRow("SELECT name FROM social_lists WHERE id = ?", listID).Scan(&listName)
			if err == sql.ErrNoRows {
				return fmt.Errorf("list %q not found", listID)
			}
			if err != nil {
				return fmt.Errorf("querying list: %w", err)
			}

			itemID := storage.NewID()
			now := time.Now().UTC()

			tx, err := db.DB.Begin()
			if err != nil {
				return fmt.Errorf("beginning transaction: %w", err)
			}

			_, err = tx.Exec(
				`INSERT INTO social_list_items (id, list_id, platform, platform_username, follower_count, created_at)
				 VALUES (?, ?, ?, ?, 0, ?)`,
				itemID, listID, strings.ToLower(platform), username, now,
			)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("adding item: %w", err)
			}

			_, err = tx.Exec(
				"UPDATE social_lists SET item_count = item_count + 1, updated_at = ? WHERE id = ?",
				now, listID,
			)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("updating list count: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing transaction: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"item_id":  itemID,
					"list_id":  listID,
					"username": username,
					"platform": platform,
				})
			}

			fmt.Fprintf(os.Stdout, "Added %s (%s) to list %s (%s)\n", username, platform, listID, listName)
			return nil
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "Platform username (required)")
	cmd.Flags().StringVar(&platform, "platform", "", "Platform name (required)")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("platform")

	return cmd
}
