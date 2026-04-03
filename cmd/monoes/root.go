package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/nokhodian/mono-agent/internal/storage"
	"github.com/spf13/cobra"
)

type globalConfig struct {
	DBPath     string
	OutputDir  string
	ConfigDir  string
	Headless   bool
	Workers    int
	Verbose    bool
	JSONOutput bool
	LogFile    string
}

func newRootCmd() *cobra.Command {
	cfg := &globalConfig{}

	cmd := &cobra.Command{
		Use:           "monoes",
		Short:         "Multi-platform social media automation agent",
		Long:          "Mono Agent — automate keyword search, profile discovery, bulk messaging, content publishing, and more across Instagram, LinkedIn, X, TikTok, Telegram, and Email.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	cmd.PersistentFlags().StringVar(&cfg.DBPath, "db-path", "~/.monoes/monoes.db", "SQLite database path")
	cmd.PersistentFlags().StringVar(&cfg.OutputDir, "output-dir", "~/.monoes/output", "JSON file output directory")
	cmd.PersistentFlags().StringVar(&cfg.ConfigDir, "config-dir", "~/.monoes/configs", "XPath config cache directory")
	cmd.PersistentFlags().BoolVar(&cfg.Headless, "headless", false, "Run browser in headless mode")
	cmd.PersistentFlags().IntVar(&cfg.Workers, "workers", 1, "Number of concurrent browser workers")
	cmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable debug logging")
	cmd.PersistentFlags().BoolVar(&cfg.JSONOutput, "json", false, "Output in JSON format")
	cmd.PersistentFlags().StringVar(&cfg.LogFile, "log-file", "", "Path to log file")

	// Register subcommands
	cmd.AddCommand(
		newLoginCmd(cfg),
		newLogoutCmd(cfg),
		newRunCmd(cfg),
		newSearchCmd(cfg),
		newMessageCmd(cfg),
		newCommentCmd(cfg),
		newActionCmd(cfg),
		newPeopleCmd(cfg),
		newListCmd(cfg),
		newTemplateCmd(cfg),
		newConfigCmd(cfg),
		newScheduleCmd(cfg),
		newExportCmd(cfg),
		newStatusCmd(cfg),
		newVersionCmd(),
		newWorkflowCmd(cfg),
		newNodeCmd(cfg),
		newConnectCmd(cfg),
	)

	return cmd
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// initDB creates and returns a database connection, applying migrations.
func initDB(cfg *globalConfig) (*storage.Database, error) {
	dbPath := expandPath(cfg.DBPath)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, err
	}
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.ApplyMigrations(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// ensureDir creates a directory if it does not exist.
func ensureDir(path string) error {
	expanded := expandPath(path)
	return os.MkdirAll(expanded, 0755)
}
