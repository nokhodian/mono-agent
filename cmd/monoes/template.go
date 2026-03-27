package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/monoes/monoes-agent/internal/storage"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newTemplateCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage message templates",
		Long:  "Create, list, view, and delete message templates used for bulk messaging.",
	}

	cmd.AddCommand(
		newTemplateListCmd(cfg),
		newTemplateGetCmd(cfg),
		newTemplateCreateCmd(cfg),
		newTemplateDeleteCmd(cfg),
	)

	return cmd
}

func newTemplateListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all templates",
		Example: `  monoes template list
  monoes template list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			rows, err := db.DB.Query(
				`SELECT id, name, COALESCE(subject,''), body, COALESCE(metadata,''), created_at, updated_at
				 FROM templates ORDER BY name ASC`,
			)
			if err != nil {
				return fmt.Errorf("querying templates: %w", err)
			}
			defer rows.Close()

			var templates []storage.Template
			for rows.Next() {
				var t storage.Template
				if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.Metadata, &t.CreatedAt, &t.UpdatedAt); err != nil {
					return fmt.Errorf("scanning template: %w", err)
				}
				templates = append(templates, t)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating templates: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(templates)
			}

			if len(templates) == 0 {
				fmt.Println("No templates found.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Name", "Subject", "Body", "Created"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			for _, t := range templates {
				table.Append([]string{
					fmt.Sprintf("%d", t.ID),
					truncateStr(t.Name, 20),
					truncateStr(t.Subject, 20),
					truncateStr(t.Body, 30),
					t.CreatedAt.Format("2006-01-02 15:04"),
				})
			}
			table.Render()
			fmt.Fprintf(os.Stderr, "\nTotal: %d template(s)\n", len(templates))
			return nil
		},
	}
}

func newTemplateGetCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show template details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var templateID int
			if _, err := fmt.Sscanf(args[0], "%d", &templateID); err != nil {
				return fmt.Errorf("invalid template ID %q: must be an integer", args[0])
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			var t storage.Template
			var subject, metadata sql.NullString
			err = db.DB.QueryRow(
				`SELECT id, name, subject, body, metadata, created_at, updated_at
				 FROM templates WHERE id = ?`, templateID,
			).Scan(&t.ID, &t.Name, &subject, &t.Body, &metadata, &t.CreatedAt, &t.UpdatedAt)
			if err == sql.ErrNoRows {
				return fmt.Errorf("template %d not found", templateID)
			}
			if err != nil {
				return fmt.Errorf("querying template: %w", err)
			}
			t.Subject = subject.String
			t.Metadata = metadata.String

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(t)
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Field", "Value"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)
			table.SetColumnAlignment([]int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT})

			table.Append([]string{"ID", fmt.Sprintf("%d", t.ID)})
			table.Append([]string{"Name", t.Name})
			table.Append([]string{"Subject", t.Subject})
			table.Append([]string{"Body", t.Body})
			if t.Metadata != "" {
				table.Append([]string{"Metadata", truncateStr(t.Metadata, 60)})
			}
			table.Append([]string{"Created", t.CreatedAt.Format("2006-01-02 15:04:05")})
			table.Append([]string{"Updated", t.UpdatedAt.Format("2006-01-02 15:04:05")})
			table.Render()

			return nil
		},
	}
}

func newTemplateCreateCmd(cfg *globalConfig) *cobra.Command {
	var (
		name    string
		body    string
		subject string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new template",
		Example: `  monoes template create --name "Welcome" --body "Hello {{name}}!" --subject "Welcome aboard"
  monoes template create --name "Follow-up" --body "Just checking in..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if body == "" {
				return fmt.Errorf("--body is required")
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			now := time.Now().UTC()

			var subjectVal sql.NullString
			if subject != "" {
				subjectVal = sql.NullString{String: subject, Valid: true}
			}

			result, err := db.DB.Exec(
				`INSERT INTO templates (name, subject, body, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				name, subjectVal, body, now, now,
			)
			if err != nil {
				return fmt.Errorf("creating template: %w", err)
			}

			id, _ := result.LastInsertId()

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"id":      id,
					"name":    name,
					"subject": subject,
					"body":    body,
				})
			}

			fmt.Fprintf(os.Stdout, "Created template %d (%s)\n", id, name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Template name (required)")
	cmd.Flags().StringVar(&body, "body", "", "Template body (required)")
	cmd.Flags().StringVar(&subject, "subject", "", "Template subject (optional)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}

func newTemplateDeleteCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var templateID int
			if _, err := fmt.Sscanf(args[0], "%d", &templateID); err != nil {
				return fmt.Errorf("invalid template ID %q: must be an integer", args[0])
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			result, err := db.DB.Exec("DELETE FROM templates WHERE id = ?", templateID)
			if err != nil {
				return fmt.Errorf("deleting template: %w", err)
			}

			affected, _ := result.RowsAffected()
			if affected == 0 {
				return fmt.Errorf("template %d not found", templateID)
			}

			fmt.Fprintf(os.Stdout, "Deleted template %d.\n", templateID)
			return nil
		},
	}
}
