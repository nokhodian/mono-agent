package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/monoes/monoes-agent/internal/bot"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	// Import platform bots to trigger init() registration.
	_ "github.com/monoes/monoes-agent/internal/bot/email"
	_ "github.com/monoes/monoes-agent/internal/bot/instagram"
	_ "github.com/monoes/monoes-agent/internal/bot/linkedin"
	_ "github.com/monoes/monoes-agent/internal/bot/telegram"
	_ "github.com/monoes/monoes-agent/internal/bot/tiktok"
	_ "github.com/monoes/monoes-agent/internal/bot/x"
)

func newLoginCmd(cfg *globalConfig) *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "login <platform>",
		Short: "Login to a social platform via browser",
		Long:  "Opens a browser window for the specified platform and waits for you to log in manually. Cookies are saved to the database upon successful login.",
		Example: `  monoes login instagram
  monoes login linkedin --timeout 5m
  monoes login x --headless=false`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform := strings.ToUpper(args[0])

			factory, ok := bot.PlatformRegistry[platform]
			if !ok {
				supported := make([]string, 0, len(bot.PlatformRegistry))
				for k := range bot.PlatformRegistry {
					supported = append(supported, strings.ToLower(k))
				}
				return fmt.Errorf("unsupported platform %q; supported: %s", args[0], strings.Join(supported, ", "))
			}

			adapter := factory()

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			// Launch browser.
			launchURL, err := launcher.New().
				Headless(cfg.Headless).
				Launch()
			if err != nil {
				return fmt.Errorf("launching browser: %w", err)
			}

			browser := rod.New().ControlURL(launchURL)
			if err := browser.Connect(); err != nil {
				return fmt.Errorf("connecting to browser: %w", err)
			}
			defer browser.Close()

			page, err := browser.Page(proto.TargetCreateTarget{URL: adapter.LoginURL()})
			if err != nil {
				return fmt.Errorf("navigating to login page: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Please log in to %s manually in the browser window...\n", platform)
			fmt.Fprintf(os.Stderr, "Waiting for login (timeout: %s)...\n", timeout)

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return fmt.Errorf("login timed out after %s", timeout)
				case <-ticker.C:
					loggedIn, checkErr := adapter.IsLoggedIn(page)
					if checkErr != nil {
						continue
					}
					if loggedIn {
						// Extract cookies and save to database.
						cookies, cookieErr := page.Cookies(nil)
						if cookieErr != nil {
							return fmt.Errorf("extracting cookies: %w", cookieErr)
						}

						cookiesJSON, marshalErr := json.Marshal(cookies)
						if marshalErr != nil {
							return fmt.Errorf("marshalling cookies: %w", marshalErr)
						}

						username := adapter.ExtractUsername(page.MustInfo().URL)
						if username == "" {
							username = "unknown"
						}

						expiry := time.Now().Add(30 * 24 * time.Hour) // 30 days
						_, dbErr := db.DB.Exec(
							`INSERT OR REPLACE INTO crawler_sessions (username, platform, cookies_json, expiry)
							 VALUES (?, ?, ?, ?)`,
							username, strings.ToLower(platform), string(cookiesJSON), expiry,
						)
						if dbErr != nil {
							return fmt.Errorf("saving session: %w", dbErr)
						}

						fmt.Fprintf(os.Stderr, "Login successful for %s (user: %s). Session saved.\n", platform, username)
						return nil
					}
				}
			}
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 3*time.Minute, "Maximum time to wait for login")

	// Subcommand: login status
	cmd.AddCommand(newLoginStatusCmd(cfg))

	return cmd
}

func newLoginStatusCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show login status for all platforms",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			rows, err := db.DB.Query(
				`SELECT id, username, platform, expiry, when_added FROM crawler_sessions ORDER BY platform`,
			)
			if err != nil {
				return fmt.Errorf("querying sessions: %w", err)
			}
			defer rows.Close()

			type sessionRow struct {
				ID        int
				Username  string
				Platform  string
				Expiry    time.Time
				WhenAdded time.Time
			}

			var sessions []sessionRow
			for rows.Next() {
				var s sessionRow
				if err := rows.Scan(&s.ID, &s.Username, &s.Platform, &s.Expiry, &s.WhenAdded); err != nil {
					return fmt.Errorf("scanning session row: %w", err)
				}
				sessions = append(sessions, s)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating sessions: %w", err)
			}

			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(sessions)
			}

			if len(sessions) == 0 {
				fmt.Println("No active sessions found.")
				return nil
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Platform", "Username", "Status", "Expires", "Added"})
			table.SetBorder(false)
			table.SetAutoWrapText(false)

			now := time.Now()
			for _, s := range sessions {
				status := "active"
				if s.Expiry.Before(now) {
					status = "expired"
				}
				table.Append([]string{
					fmt.Sprintf("%d", s.ID),
					s.Platform,
					s.Username,
					status,
					s.Expiry.Format("2006-01-02 15:04"),
					s.WhenAdded.Format("2006-01-02 15:04"),
				})
			}
			table.Render()
			return nil
		},
	}
}

func newLogoutCmd(cfg *globalConfig) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "logout [platform]",
		Short: "Delete saved session for a platform",
		Long:  "Removes saved cookies/session for the specified platform. Use --all to remove all sessions.",
		Example: `  monoes logout instagram
  monoes logout --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			if all {
				result, err := db.DB.Exec("DELETE FROM crawler_sessions")
				if err != nil {
					return fmt.Errorf("deleting all sessions: %w", err)
				}
				count, _ := result.RowsAffected()
				fmt.Fprintf(os.Stderr, "Deleted %d session(s).\n", count)
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("specify a platform or use --all")
			}

			platform := strings.ToLower(args[0])
			result, err := db.DB.Exec("DELETE FROM crawler_sessions WHERE platform = ?", platform)
			if err != nil {
				return fmt.Errorf("deleting session for %s: %w", platform, err)
			}
			count, _ := result.RowsAffected()
			if count == 0 {
				fmt.Fprintf(os.Stderr, "No session found for %s.\n", platform)
			} else {
				fmt.Fprintf(os.Stderr, "Deleted session for %s.\n", platform)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Delete all sessions")

	return cmd
}
