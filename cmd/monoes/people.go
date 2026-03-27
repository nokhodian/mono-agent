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

func newPeopleCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "people",
		Aliases: []string{"person"},
		Short:   "Manage discovered people/profiles",
		Long:    "List, view, delete, and import people discovered during search and interaction actions.",
	}

	cmd.AddCommand(
		newPeopleListCmd(cfg),
		newPeopleGetCmd(cfg),
		newPeopleDeleteCmd(cfg),
		newPeopleImportCmd(cfg),
	)

	return cmd
}

func newPeopleListCmd(cfg *globalConfig) *cobra.Command {
	var (
		platform string
		limit    int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List people in the database",
		Example: `  monoes people list
  monoes people list --platform instagram --limit 20
  monoes people list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			query := `SELECT id, platform_username, platform, COALESCE(full_name,''),
			                 COALESCE(follower_count,''), COALESCE(following_count,0), is_verified,
			                 COALESCE(category,''), COALESCE(job_title,'')
			          FROM people WHERE 1=1`
			var params []interface{}

			if platform != "" {
				query += " AND platform = ?"
				params = append(params, strings.ToLower(platform))
			}

			query += " ORDER BY created_at DESC"

			if limit > 0 {
				query += fmt.Sprintf(" LIMIT %d", limit)
			}

			rows, err := db.DB.Query(query, params...)
			if err != nil {
				return fmt.Errorf("querying people: %w", err)
			}
			defer rows.Close()

			type personSummary struct {
				ID               string `json:"id"`
				PlatformUsername string `json:"platform_username"`
				Platform         string `json:"platform"`
				FullName         string `json:"full_name"`
				FollowerCount    string `json:"follower_count"`
				FollowingCount   int    `json:"following_count"`
				IsVerified       bool   `json:"is_verified"`
				Category         string `json:"category,omitempty"`
				JobTitle         string `json:"job_title,omitempty"`
			}

			var people []personSummary
			for rows.Next() {
				var p personSummary
				var verified int
				if err := rows.Scan(
					&p.ID, &p.PlatformUsername, &p.Platform, &p.FullName,
					&p.FollowerCount, &p.FollowingCount,
					&verified, &p.Category, &p.JobTitle,
				); err != nil {
					return fmt.Errorf("scanning person: %w", err)
				}
				p.IsVerified = verified != 0
				people = append(people, p)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating people: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(people)
			}

			if len(people) == 0 {
				fmt.Println("No people found.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Username", "Platform", "Name", "Followers", "Verified"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, p := range people {
				shortID := p.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				verifiedStr := ""
				if p.IsVerified {
					verifiedStr = "yes"
				}
				table.Append([]string{
					shortID,
					truncateStr(p.PlatformUsername, 20),
					p.Platform,
					truncateStr(p.FullName, 20),
					p.FollowerCount,
					verifiedStr,
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d person(s)\n", len(people))
			return nil
		},
	}

	cmd.Flags().StringVarP(&platform, "platform", "p", "", "Filter by platform")
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "Maximum number of results")

	return cmd
}

func newPeopleGetCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show detailed information about a person",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			personID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			var p storage.Person
			var verified int
			var fullName, imageURL, contactDetails, website sql.NullString
			var followerCount, introduction, category, jobTitle sql.NullString

			err = db.DB.QueryRow(
				`SELECT id, platform_username, platform, full_name,
				        image_url, contact_details,
				        website, content_count, follower_count,
				        following_count, introduction, is_verified,
				        category, job_title,
				        created_at, updated_at
				 FROM people WHERE id = ?`, personID,
			).Scan(
				&p.ID, &p.PlatformUsername, &p.Platform, &fullName,
				&imageURL, &contactDetails, &website, &p.ContentCount,
				&followerCount, &p.FollowingCount, &introduction,
				&verified, &category, &jobTitle, &p.CreatedAt, &p.UpdatedAt,
			)
			if err == sql.ErrNoRows {
				return fmt.Errorf("person %q not found", personID)
			}
			if err != nil {
				return fmt.Errorf("querying person: %w", err)
			}

			p.FullName = fullName.String
			p.ImageURL = imageURL.String
			p.ContactDetails = contactDetails.String
			p.Website = website.String
			p.FollowerCount = followerCount.String
			p.Introduction = introduction.String
			p.IsVerified = verified != 0
			p.Category = category.String
			p.JobTitle = jobTitle.String

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(p)
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Field", "Value"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)
			table.SetColumnAlignment([]int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT})

			table.Append([]string{"ID", p.ID})
			table.Append([]string{"Username", p.PlatformUsername})
			table.Append([]string{"Platform", p.Platform})
			table.Append([]string{"Full Name", p.FullName})
			table.Append([]string{"Verified", fmt.Sprintf("%v", p.IsVerified)})
			table.Append([]string{"Category", p.Category})
			table.Append([]string{"Job Title", p.JobTitle})
			table.Append([]string{"Followers", p.FollowerCount})
			table.Append([]string{"Following", fmt.Sprintf("%d", p.FollowingCount)})
			table.Append([]string{"Content Count", fmt.Sprintf("%d", p.ContentCount)})
			if p.Website != "" {
				table.Append([]string{"Website", p.Website})
			}
			if p.ContactDetails != "" {
				table.Append([]string{"Contact", p.ContactDetails})
			}
			if p.Introduction != "" {
				table.Append([]string{"Bio", truncateStr(p.Introduction, 60)})
			}
			if p.ImageURL != "" {
				table.Append([]string{"Image", truncateStr(p.ImageURL, 60)})
			}
			table.Append([]string{"Created", p.CreatedAt.Format("2006-01-02 15:04:05")})
			table.Append([]string{"Updated", p.UpdatedAt.Format("2006-01-02 15:04:05")})
			table.Render()

			return nil
		},
	}
}

func newPeopleDeleteCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a person from the database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			personID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			result, err := db.DB.Exec("DELETE FROM people WHERE id = ?", personID)
			if err != nil {
				return fmt.Errorf("deleting person: %w", err)
			}

			affected, _ := result.RowsAffected()
			if affected == 0 {
				return fmt.Errorf("person %q not found", personID)
			}

			fmt.Fprintf(os.Stdout, "Deleted person %s.\n", personID)
			return nil
		},
	}
}

func newPeopleImportCmd(cfg *globalConfig) *cobra.Command {
	var (
		filePath string
		platform string
	)

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import people from a JSON array file",
		Example: `  monoes people import --file people.json --platform instagram
  monoes people import --file contacts.json --platform linkedin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			if platform == "" {
				return fmt.Errorf("--platform is required")
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

			var rawPeople []map[string]interface{}
			if err := json.Unmarshal(data, &rawPeople); err != nil {
				return fmt.Errorf("parsing JSON array: %w", err)
			}

			if len(rawPeople) == 0 {
				fmt.Fprintln(os.Stderr, "No people found in file.")
				return nil
			}

			tx, err := db.DB.Begin()
			if err != nil {
				return fmt.Errorf("beginning transaction: %w", err)
			}

			stmt, err := tx.Prepare(
				`INSERT INTO people (id, platform_username, platform, full_name, image_url,
				        contact_details, website, content_count, follower_count,
				        following_count, introduction, is_verified, category, job_title,
				        created_at, updated_at)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
				 ON CONFLICT(platform_username, platform)
				 DO UPDATE SET
				   full_name       = excluded.full_name,
				   image_url       = excluded.image_url,
				   contact_details = excluded.contact_details,
				   website         = excluded.website,
				   content_count   = excluded.content_count,
				   follower_count  = excluded.follower_count,
				   following_count = excluded.following_count,
				   introduction    = excluded.introduction,
				   is_verified     = excluded.is_verified,
				   category        = excluded.category,
				   job_title       = excluded.job_title,
				   updated_at      = excluded.updated_at`,
			)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("preparing statement: %w", err)
			}
			defer stmt.Close()

			now := time.Now().UTC()
			var imported int

			for _, raw := range rawPeople {
				username, _ := raw["platform_username"].(string)
				if username == "" {
					username, _ = raw["username"].(string)
				}
				if username == "" {
					continue
				}

				fullName, _ := raw["full_name"].(string)
				imageURL, _ := raw["image_url"].(string)
				contactDetails, _ := raw["contact_details"].(string)
				website, _ := raw["website"].(string)
				introduction, _ := raw["introduction"].(string)
				category, _ := raw["category"].(string)
				jobTitle, _ := raw["job_title"].(string)
				followerCount, _ := raw["follower_count"].(string)

				var contentCount, followingCount int
				if v, ok := raw["content_count"].(float64); ok {
					contentCount = int(v)
				}
				if v, ok := raw["following_count"].(float64); ok {
					followingCount = int(v)
				}

				var isVerified int
				if v, ok := raw["is_verified"].(bool); ok && v {
					isVerified = 1
				}

				personID := storage.NewID()
				if id, ok := raw["id"].(string); ok && id != "" {
					personID = id
				}

				_, err := stmt.Exec(
					personID, username, strings.ToLower(platform),
					nullableStr(fullName), nullableStr(imageURL),
					nullableStr(contactDetails), nullableStr(website),
					contentCount, nullableStr(followerCount), followingCount,
					nullableStr(introduction), isVerified,
					nullableStr(category), nullableStr(jobTitle),
					now, now,
				)
				if err != nil {
					tx.Rollback()
					return fmt.Errorf("importing person %s: %w", username, err)
				}
				imported++
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing import: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Imported %d person(s) for platform %s.\n", imported, platform)
			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to JSON file containing people array (required)")
	cmd.Flags().StringVar(&platform, "platform", "", "Platform for imported people (required)")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagRequired("platform")

	return cmd
}

// nullableStr returns a sql.NullString; empty strings map to NULL.
func nullableStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
