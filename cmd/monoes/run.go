package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/nokhodian/mono-agent/internal/action"
	"github.com/nokhodian/mono-agent/internal/bot"
	browserpkg "github.com/nokhodian/mono-agent/internal/browser"
	"github.com/nokhodian/mono-agent/internal/config"
	"github.com/nokhodian/mono-agent/internal/extension"
	"github.com/nokhodian/mono-agent/internal/storage"
	"github.com/nokhodian/mono-agent/internal/util"
	"github.com/olekukonko/tablewriter"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newRunCmd(cfg *globalConfig) *cobra.Command {
	var (
		filePath string
		queue    bool
		watch    bool
		interval time.Duration
		timeout  time.Duration
		params   []string
	)

	cmd := &cobra.Command{
		Use:   "run [action-id]",
		Short: "Execute one or more actions",
		Long: `Run actions by ID, from a JSON file, or from the pending queue.

When --watch is enabled, the runner continuously polls for new pending
actions and executes them at the specified interval.`,
		Example: `  monoes run abc-123
  monoes run --file actions.json
  monoes run --queue
  monoes run --watch --interval 30s`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}
			defer db.Close()

			ctx := cmd.Context()

			// Parse --param key=value flags into a map.
			paramMap := make(map[string]interface{})
			for _, p := range params {
				parts := strings.SplitN(p, "=", 2)
				if len(parts) == 2 {
					paramMap[parts[0]] = parts[1]
				}
			}

			// Determine which actions to run.
			switch {
			case len(args) == 1:
				// Run single action by ID.
				actionID := args[0]
				act, err := loadActionByID(db, actionID)
				if err != nil {
					return err
				}
				if len(paramMap) > 0 {
					if act.Params == nil {
						act.Params = make(map[string]interface{})
					}
					for k, v := range paramMap {
						act.Params[k] = v
					}
				}
				return executeAction(ctx, db, cfg, act, timeout)

			case filePath != "":
				// Run from JSON file.
				data, err := os.ReadFile(filePath)
				if err != nil {
					return fmt.Errorf("reading file %s: %w", filePath, err)
				}

				act, err := storage.ParseAction(data)
				if err != nil {
					return fmt.Errorf("parsing action from file: %w", err)
				}

				// Upsert into database.
				if err := upsertAction(db, act); err != nil {
					return fmt.Errorf("saving action to database: %w", err)
				}

				fmt.Fprintf(os.Stderr, "Loaded action %s (%s) from file\n", act.ID, act.Type)
				return executeAction(ctx, db, cfg, act, timeout)

			case watch:
				// Continuously poll and execute.
				fmt.Fprintf(os.Stderr, "Watching for pending actions (interval: %s)...\n", interval)
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						fmt.Fprintln(os.Stderr, "Watch mode interrupted.")
						return nil
					case <-ticker.C:
						actions, err := loadPendingActions(db)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error loading pending actions: %v\n", err)
							continue
						}
						for _, act := range actions {
							select {
							case <-ctx.Done():
								return nil
							default:
							}
							if err := executeAction(ctx, db, cfg, act, timeout); err != nil {
								fmt.Fprintf(os.Stderr, "Action %s failed: %v\n", act.ID, err)
							}
						}
					}
				}

			case queue:
				// Run all pending actions once.
				actions, err := loadPendingActions(db)
				if err != nil {
					return err
				}
				if len(actions) == 0 {
					fmt.Fprintln(os.Stderr, "No pending actions in queue.")
					return nil
				}
				fmt.Fprintf(os.Stderr, "Running %d pending action(s)...\n", len(actions))
				var failed int
				for _, act := range actions {
					select {
					case <-ctx.Done():
						return fmt.Errorf("interrupted after %d of %d actions", failed, len(actions))
					default:
					}
					if err := executeAction(ctx, db, cfg, act, timeout); err != nil {
						fmt.Fprintf(os.Stderr, "Action %s failed: %v\n", act.ID, err)
						failed++
					}
				}
				fmt.Fprintf(os.Stderr, "Completed: %d succeeded, %d failed out of %d total.\n",
					len(actions)-failed, failed, len(actions))
				return nil

			default:
				return fmt.Errorf("specify an action ID, --file, --queue, or --watch")
			}
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to action JSON file")
	cmd.Flags().BoolVar(&queue, "queue", false, "Run all pending actions from the queue")
	cmd.Flags().BoolVar(&watch, "watch", false, "Continuously poll and execute pending actions")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "Polling interval for --watch mode")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum execution time per action")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Set a custom param (key=value, repeatable)")

	return cmd
}

// loadActionByID fetches a single action from the database.
func loadActionByID(db *storage.Database, id string) (*storage.Action, error) {
	row := db.DB.QueryRow(
		`SELECT id, created_at, title, type, state, disabled, target_platform,
		        position, content_subject, content_message, content_blob_urls,
		        scheduled_date, execution_interval, start_date, end_date,
		        campaign_id, reached_index, keywords, action_execution_count,
		        COALESCE(params,'{}')
		 FROM actions WHERE id = ?`, id,
	)

	var a storage.Action
	var disabled int
	var contentSubject, contentMessage, contentBlobURLs sql.NullString
	var scheduledDate, startDate, endDate, campaignID, keywords sql.NullString
	var executionInterval sql.NullInt64
	var paramsJSON string

	err := row.Scan(
		&a.ID, &a.CreatedAt, &a.Title, &a.Type, &a.State, &disabled, &a.TargetPlatform,
		&a.Position, &contentSubject, &contentMessage, &contentBlobURLs,
		&scheduledDate, &executionInterval, &startDate, &endDate,
		&campaignID, &a.ReachedIndex, &keywords, &a.ActionExecutionCount,
		&paramsJSON,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("action %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying action %q: %w", id, err)
	}

	a.Disabled = disabled != 0
	a.ContentSubject = contentSubject.String
	a.ContentMessage = contentMessage.String
	a.ContentBlobURLs = contentBlobURLs.String
	a.ScheduledDate = scheduledDate.String
	a.StartDate = startDate.String
	a.EndDate = endDate.String
	a.CampaignID = campaignID.String
	a.Keywords = keywords.String
	if executionInterval.Valid {
		a.ExecutionInterval = int(executionInterval.Int64)
	}
	if paramsJSON != "" && paramsJSON != "{}" {
		var p map[string]interface{}
		if json.Unmarshal([]byte(paramsJSON), &p) == nil {
			a.Params = p
		}
	}
	if a.Params == nil {
		a.Params = make(map[string]interface{})
	}

	return &a, nil
}

// loadPendingActions fetches all actions in PENDING state ordered by position.
func loadPendingActions(db *storage.Database) ([]*storage.Action, error) {
	rows, err := db.DB.Query(
		`SELECT id, created_at, title, type, state, disabled, target_platform,
		        position, content_subject, content_message, content_blob_urls,
		        scheduled_date, execution_interval, start_date, end_date,
		        campaign_id, reached_index, keywords, action_execution_count,
		        COALESCE(params,'{}')
		 FROM actions WHERE state = 'PENDING' AND disabled = 0
		 ORDER BY position ASC, created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying pending actions: %w", err)
	}
	defer rows.Close()

	var actions []*storage.Action
	for rows.Next() {
		var a storage.Action
		var disabled int
		var contentSubject, contentMessage, contentBlobURLs sql.NullString
		var scheduledDate, startDate, endDate, campaignID, keywords sql.NullString
		var executionInterval sql.NullInt64
		var paramsJSON string

		if err := rows.Scan(
			&a.ID, &a.CreatedAt, &a.Title, &a.Type, &a.State, &disabled, &a.TargetPlatform,
			&a.Position, &contentSubject, &contentMessage, &contentBlobURLs,
			&scheduledDate, &executionInterval, &startDate, &endDate,
			&campaignID, &a.ReachedIndex, &keywords, &a.ActionExecutionCount,
			&paramsJSON,
		); err != nil {
			return nil, fmt.Errorf("scanning action row: %w", err)
		}

		a.Disabled = disabled != 0
		a.ContentSubject = contentSubject.String
		a.ContentMessage = contentMessage.String
		a.ContentBlobURLs = contentBlobURLs.String
		a.ScheduledDate = scheduledDate.String
		a.StartDate = startDate.String
		a.EndDate = endDate.String
		a.CampaignID = campaignID.String
		a.Keywords = keywords.String
		if executionInterval.Valid {
			a.ExecutionInterval = int(executionInterval.Int64)
		}
		if paramsJSON != "" && paramsJSON != "{}" {
			var p map[string]interface{}
			if json.Unmarshal([]byte(paramsJSON), &p) == nil {
				a.Params = p
			}
		}
		if a.Params == nil {
			a.Params = make(map[string]interface{})
		}

		actions = append(actions, &a)
	}
	return actions, rows.Err()
}

// upsertAction inserts or replaces an action in the database.
func upsertAction(db *storage.Database, a *storage.Action) error {
	disabled := 0
	if a.Disabled {
		disabled = 1
	}
	paramsJSON := "{}"
	if len(a.Params) > 0 {
		if b, err := json.Marshal(a.Params); err == nil {
			paramsJSON = string(b)
		}
	}
	_, err := db.DB.Exec(
		`INSERT OR REPLACE INTO actions
		 (id, created_at, title, type, state, disabled, target_platform, position,
		  content_subject, content_message, content_blob_urls,
		  scheduled_date, execution_interval, start_date, end_date,
		  campaign_id, reached_index, keywords, action_execution_count, params, updated_at_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		a.ID, a.CreatedAt, a.Title, a.Type, a.State, disabled, a.TargetPlatform, a.Position,
		a.ContentSubject, a.ContentMessage, a.ContentBlobURLs,
		a.ScheduledDate, a.ExecutionInterval, a.StartDate, a.EndDate,
		a.CampaignID, a.ReachedIndex, a.Keywords, a.ActionExecutionCount, paramsJSON,
	)
	return err
}

// ---------------------------------------------------------------------------
// Storage adapter — implements action.StorageInterface
// ---------------------------------------------------------------------------

type storageAdapter struct {
	db *storage.Database
}

func (s *storageAdapter) UpdateActionState(id, state string) error {
	_, err := s.db.DB.Exec(
		"UPDATE actions SET state = ?, updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
		state, id,
	)
	return err
}

func (s *storageAdapter) UpdateActionReachedIndex(id string, index int) error {
	_, err := s.db.DB.Exec(
		"UPDATE actions SET reached_index = ?, updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
		index, id,
	)
	return err
}

func (s *storageAdapter) SaveExtractedData(actionID string, items []map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := s.db.DB.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO people (id, platform_username, platform, full_name, image_url,
		        contact_details, website, content_count, follower_count,
		        following_count, introduction, is_verified, category, job_title,
		        profile_url, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(platform_username, platform)
		 DO UPDATE SET
		   full_name       = COALESCE(excluded.full_name, people.full_name),
		   image_url       = COALESCE(excluded.image_url, people.image_url),
		   profile_url     = COALESCE(excluded.profile_url, people.profile_url),
		   website         = COALESCE(excluded.website, people.website),
		   content_count   = COALESCE(excluded.content_count, people.content_count),
		   follower_count  = COALESCE(excluded.follower_count, people.follower_count),
		   following_count = COALESCE(excluded.following_count, people.following_count),
		   introduction    = COALESCE(excluded.introduction, people.introduction),
		   is_verified     = COALESCE(excluded.is_verified, people.is_verified),
		   updated_at      = excluded.updated_at`,
	)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC()

	for _, item := range items {
		platform, _ := item["platform"].(string)
		profileURL, _ := item["url"].(string)

		// Extract username from profile URL.
		username := ""
		platformUpper := strings.ToUpper(platform)
		if factory, ok := bot.PlatformRegistry[platformUpper]; ok {
			adapter := factory()
			username = adapter.ExtractUsername(profileURL)
		}
		if username == "" {
			// Fallback: try to get from the URL path directly.
			if profileURL != "" {
				parts := strings.Split(strings.Trim(profileURL, "/"), "/")
				if len(parts) > 0 {
					last := parts[len(parts)-1]
					username = strings.TrimPrefix(last, "@")
				}
			}
		}
		if username == "" {
			continue // Can't save without a username.
		}

		fullName, _ := item["full_name"].(string)
		imageURL, _ := item["image_url"].(string)
		website, _ := item["website"].(string)
		followerCountRaw, _ := item["follower_count"].(string)
		followingCountRaw, _ := item["following_count"].(string)
		contentCountRaw, _ := item["content_count"].(string)
		introduction, _ := item["introduction"].(string)

		// Strip word suffixes like "followers", "posts" before parsing.
		contentCount := extractNumericPart(contentCountRaw)
		followerCount := extractNumericPart(followerCountRaw)
		followingCount := extractNumericPart(followingCountRaw)

		var contentCountInt int64
		if contentCount != "" {
			contentCountInt, _ = util.ConvertAbbreviatedNumber(contentCount)
		}
		var followingCountInt int64
		if followingCount != "" {
			followingCountInt, _ = util.ConvertAbbreviatedNumber(followingCount)
		}

		var isVerified int
		if v, ok := item["is_verified"].(string); ok && v != "" && v != "false" && v != "<nil>" {
			isVerified = 1
		}
		if v, ok := item["is_verified"].(bool); ok && v {
			isVerified = 1
		}

		personID := storage.NewID()
		_, err := stmt.Exec(
			personID, username, strings.ToUpper(platform),
			nullableStr(fullName), nullableStr(imageURL),
			sql.NullString{}, // contact_details
			nullableStr(website),
			contentCountInt,
			nullableStr(followerCount),
			followingCountInt,
			nullableStr(introduction),
			isVerified,
			sql.NullString{}, // category
			sql.NullString{}, // job_title
			nullableStr(profileURL),
			now, now,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to save person %s: %v\n", username, err)
			continue
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Browser + session management
// ---------------------------------------------------------------------------

// launchBrowserPage opens the user's real Chrome profile so existing logins
// (Google, Instagram, etc.) are available without cookie restoration.
func launchBrowserPage(cfg *globalConfig, db *storage.Database, platform string) (*rod.Browser, *rod.Page, error) {
	l := launcher.New().
		Headless(cfg.Headless).
		Set("disable-blink-features", "AutomationControlled")

	launchURL, err := l.Launch()
	if err != nil {
		return nil, nil, fmt.Errorf("launching browser: %w", err)
	}

	browser := rod.New().ControlURL(launchURL)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connecting to browser: %w", err)
	}

	// Navigate to platform domain first, then restore cookies, then reload.
	platformDomains := map[string]string{
		"gemini":    "https://gemini.google.com/app",
		"instagram": "https://www.instagram.com",
		"linkedin":  "https://www.linkedin.com",
		"x":         "https://x.com",
		"tiktok":    "https://www.tiktok.com",
	}
	startURL := "about:blank"
	if domain, ok := platformDomains[strings.ToLower(platform)]; ok {
		startURL = domain
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: startURL})
	if err != nil {
		browser.Close()
		return nil, nil, fmt.Errorf("creating page: %w", err)
	}
	time.Sleep(5 * time.Second)

	// Restore cookies from DB and reload.
	platformLower := strings.ToLower(platform)
	var cookiesJSON string
	err = db.DB.QueryRow(
		"SELECT cookies_json FROM crawler_sessions WHERE platform = ? ORDER BY expiry DESC LIMIT 1",
		platformLower,
	).Scan(&cookiesJSON)
	if err == nil && cookiesJSON != "" {
		var cookies []*proto.NetworkCookieParam
		if jsonErr := json.Unmarshal([]byte(cookiesJSON), &cookies); jsonErr == nil {
			if setErr := page.SetCookies(cookies); setErr == nil {
				fmt.Fprintf(os.Stderr, "  Session cookies restored for %s\n", platform)
				_ = page.Reload()
				time.Sleep(5 * time.Second)
			}
		}
	}

	return browser, page, nil
}

// ---------------------------------------------------------------------------
// executeAction — the real execution pipeline
// ---------------------------------------------------------------------------

// executeAction transitions an action through RUNNING -> COMPLETED/FAILED.
// It launches a browser, restores cookies, and uses the ActionExecutor to
// run the embedded JSON action definition step-by-step.
func executeAction(
	ctx context.Context,
	db *storage.Database,
	cfg *globalConfig,
	act *storage.Action,
	timeout time.Duration,
) error {
	fmt.Fprintf(os.Stderr, "--- Executing action %s [%s] on %s ---\n", act.ID, act.Type, act.TargetPlatform)

	startTime := time.Now()

	// Check if an action definition exists for this platform/type.
	loader := action.GetLoader()
	_, defErr := loader.Load(act.TargetPlatform, act.Type)
	if defErr != nil {
		fmt.Fprintf(os.Stderr, "  No action definition found for %s/%s — skipping browser execution\n",
			act.TargetPlatform, act.Type)
		// Mark completed with no-op.
		if _, err := db.DB.Exec(
			"UPDATE actions SET state = 'COMPLETED', action_execution_count = action_execution_count + 1, updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
			act.ID,
		); err != nil {
			return fmt.Errorf("updating action state: %w", err)
		}
		return printActionResult(cfg, act, "COMPLETED", 0, 0, 0, time.Since(startTime))
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the logger.
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).
		With().Timestamp().Logger()
	if !cfg.Verbose {
		logger = logger.Level(zerolog.WarnLevel)
	}

	// Try Chrome extension first, fall back to Rod.
	var pageIface browserpkg.PageInterface
	var rodBrowser *rod.Browser

	extLogger := logger.With().Str("component", "extension").Logger()
	extServer := extension.NewServer(":9222", extLogger)
	extServer.StartAsync(execCtx)
	_ = extServer.WaitForConnection(15 * time.Second)

	if extServer.IsConnected() {
		fmt.Fprintln(os.Stderr, "  Chrome extension connected -- using your browser")
		platformURLs := map[string]string{
			"gemini":    "https://gemini.google.com/app",
			"instagram": "https://www.instagram.com",
			"linkedin":  "https://www.linkedin.com",
			"x":         "https://x.com",
			"tiktok":    "https://www.tiktok.com",
		}
		startURL := platformURLs[strings.ToLower(act.TargetPlatform)]
		if startURL == "" {
			startURL = "about:blank"
		}
		tabID, tabErr := extServer.CreateTab(startURL)
		if tabErr == nil {
			pageIface = extension.NewExtensionPage(extServer, tabID)
		} else {
			logger.Warn().Err(tabErr).Msg("extension tab creation failed, falling back to Rod")
		}
	} else {
		fmt.Fprintln(os.Stderr, "  Chrome extension not connected -- using Chromium with cookie restore")
	}

	if pageIface == nil {
		// Fallback to Rod.
		browser, page, err := launchBrowserPage(cfg, db, act.TargetPlatform)
		if err != nil {
			markActionFailed(db, act.ID)
			return fmt.Errorf("launching browser: %w", err)
		}
		rodBrowser = browser
		pageIface = browserpkg.NewRodPage(page)
	}
	if rodBrowser != nil {
		defer rodBrowser.Close()
	}

	// Create events channel for monitoring.
	events := make(chan action.ExecutionEvent, 100)
	go func() {
		for evt := range events {
			switch evt.Type {
			case "step_start":
				if cfg.Verbose {
					fmt.Fprintf(os.Stderr, "  [step] %s (%s)\n", evt.StepID, evt.Message)
				}
			case "loop_iteration":
				elapsed := time.Since(startTime).Truncate(time.Second)
				fmt.Fprintf(os.Stderr, "\r  [%s] %s", elapsed, evt.Message)
			case "action_complete":
				fmt.Fprintf(os.Stderr, "\n  %s\n", evt.Message)
			}
		}
	}()

	// Create the storage adapter.
	sa := &storageAdapter{db: db}

	// Try to get a bot adapter that supports call_bot_method steps.
	var botAdapter action.BotAdapter
	if factory, ok := bot.PlatformRegistry[strings.ToUpper(act.TargetPlatform)]; ok {
		adapter := factory()
		if ba, ok := adapter.(action.BotAdapter); ok {
			botAdapter = ba
		}
	}

	// Set up config resolution (3-tier: cache -> local/DB/API -> generate).
	configLogger := logger.With().Str("component", "config").Logger()
	apiClient := config.NewAPIClient(configLogger)
	dbStore := &config.DBConfigStore{DB: db}
	configMgr := config.NewConfigManager(cfg.ConfigDir, dbStore, apiClient, configLogger)
	configAdapter := &config.ConfigManagerAdapter{Mgr: configMgr}

	// Create the executor.
	executor := action.NewActionExecutor(
		execCtx,
		pageIface,
		sa,
		configAdapter,
		events,
		botAdapter,
		logger,
	)

	// Load action targets and seed selectedListItems so loop-based actions
	// (send_dms, comment_on_posts, scrape_profile_info, etc.) have data.
	targets, _ := loadActionTargets(db, act.ID)
	if len(targets) > 0 {
		var items []map[string]interface{}
		for _, t := range targets {
			item := map[string]interface{}{
				"id":       t.ID,
				"platform": t.Platform,
				"status":   t.Status,
			}
			if t.Link != "" {
				item["url"] = t.Link
				item["link"] = t.Link
			}
			if t.Metadata != "" {
				var meta map[string]interface{}
				if json.Unmarshal([]byte(t.Metadata), &meta) == nil {
					for k, v := range meta {
						item[k] = v
					}
				}
			}
			items = append(items, item)
		}
		executor.SetVariable("selectedListItems", items)
	}

	// Build the StorageAction from our storage.Action.
	storageAction := &action.StorageAction{
		ID:              act.ID,
		CreatedAt:       act.CreatedAt,
		Title:           act.Title,
		Type:            act.Type,
		State:           act.State,
		TargetPlatform:  act.TargetPlatform,
		ContentSubject:  act.ContentSubject,
		ContentMessage:  act.ContentMessage,
		ContentBlobURLs: act.ContentBlobURLs,
		Keywords:        act.Keywords,
		ReachedIndex:    act.ReachedIndex,
		Params:          act.Params,
	}

	// Execute!
	result, execErr := executor.Execute(storageAction)

	close(events)

	// Process results.
	finalState := "COMPLETED"
	extracted := 0
	failed := 0

	if result != nil {
		extracted = len(result.ExtractedItems)
		failed = len(result.FailedItems)

		// Save extracted data to people table.
		if extracted > 0 {
			if saveErr := sa.SaveExtractedData(act.ID, result.ExtractedItems); saveErr != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to save some extracted data: %v\n", saveErr)
			} else {
				fmt.Fprintf(os.Stderr, "  Saved %d extracted profile(s) to database\n", extracted)
			}
		}
	}

	if execErr != nil {
		if failed > 0 && extracted == 0 {
			finalState = "FAILED"
		}
		fmt.Fprintf(os.Stderr, "  Execution error: %v\n", execErr)
	}

	// Update final state.
	if _, err := db.DB.Exec(
		"UPDATE actions SET state = ?, reached_index = ?, action_execution_count = action_execution_count + 1, updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
		finalState, extracted+failed, act.ID,
	); err != nil {
		return fmt.Errorf("updating action final state: %w", err)
	}

	elapsed := time.Since(startTime)
	return printActionResult(cfg, act, finalState, extracted+failed, extracted, failed, elapsed)
}

// printActionResult displays the execution summary.
func printActionResult(cfg *globalConfig, act *storage.Action, state string, total, succeeded, failed int, elapsed time.Duration) error {
	elapsed = elapsed.Truncate(time.Second)

	if cfg.JSONOutput {
		result := map[string]interface{}{
			"action_id": act.ID,
			"type":      act.Type,
			"platform":  act.TargetPlatform,
			"state":     state,
			"total":     total,
			"succeeded": succeeded,
			"failed":    failed,
			"elapsed":   elapsed.String(),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	table := tablewriter.NewWriter(os.Stderr)
	table.SetHeader([]string{"Field", "Value"})
	table.SetBorder(false)
	table.SetAutoWrapText(false)
	table.Append([]string{"Action ID", act.ID})
	table.Append([]string{"Type", act.Type})
	table.Append([]string{"Platform", act.TargetPlatform})
	table.Append([]string{"Final State", state})
	table.Append([]string{"Processed", fmt.Sprintf("%d total, %d succeeded, %d failed", total, succeeded, failed)})
	table.Append([]string{"Elapsed", elapsed.String()})
	table.Render()

	return nil
}

// loadActionTargets loads all targets for an action.
func loadActionTargets(db *storage.Database, actionID string) ([]storage.ActionTarget, error) {
	rows, err := db.DB.Query(
		`SELECT id, action_id, COALESCE(person_id,''), platform, COALESCE(link,''),
		        COALESCE(source_type,''), status, COALESCE(last_interacted_at,''),
		        COALESCE(comment_text,''), COALESCE(metadata,''), created_at
		 FROM action_targets WHERE action_id = ? ORDER BY created_at ASC`, actionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying action targets: %w", err)
	}
	defer rows.Close()

	var targets []storage.ActionTarget
	for rows.Next() {
		var t storage.ActionTarget
		if err := rows.Scan(
			&t.ID, &t.ActionID, &t.PersonID, &t.Platform, &t.Link,
			&t.SourceType, &t.Status, &t.LastInteractedAt,
			&t.CommentText, &t.Metadata, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning target row: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// markActionFailed sets the action state to FAILED.
func markActionFailed(db *storage.Database, actionID string) {
	_, _ = db.DB.Exec(
		"UPDATE actions SET state = 'FAILED', updated_at_ts = CURRENT_TIMESTAMP WHERE id = ?",
		actionID,
	)
}

// numericPartRe matches the leading numeric portion (with optional K/M/B suffix)
// from strings like "33.9K followers", "181 posts", "1,863 posts".
var numericPartRe = regexp.MustCompile(`^[\d,.]+[KkMmBb]?`)

// extractNumericPart strips word suffixes from Instagram-style count strings.
// "33.9K followers" → "33.9K", "181 posts" → "181", "1,863 posts" → "1,863".
func extractNumericPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	match := numericPartRe.FindString(s)
	if match != "" {
		return match
	}
	return s
}
