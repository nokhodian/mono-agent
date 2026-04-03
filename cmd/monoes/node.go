package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/nokhodian/mono-agent/internal/action"
	"github.com/nokhodian/mono-agent/internal/ai"
	cfgpkg "github.com/nokhodian/mono-agent/internal/config"
	"github.com/nokhodian/mono-agent/internal/connections"
	ainodes "github.com/nokhodian/mono-agent/internal/ai/nodes"
	"github.com/nokhodian/mono-agent/internal/bot"
	_ "github.com/nokhodian/mono-agent/internal/bot/instagram"
	_ "github.com/nokhodian/mono-agent/internal/bot/linkedin"
	_ "github.com/nokhodian/mono-agent/internal/bot/tiktok"
	_ "github.com/nokhodian/mono-agent/internal/bot/x"
	"github.com/nokhodian/mono-agent/internal/nodes"
	"github.com/nokhodian/mono-agent/internal/nodes/comm"
	"github.com/nokhodian/mono-agent/internal/nodes/control"
	"github.com/nokhodian/mono-agent/internal/nodes/data"
	dbnodes "github.com/nokhodian/mono-agent/internal/nodes/db"
	httpnodes "github.com/nokhodian/mono-agent/internal/nodes/http"
	peoplenodes "github.com/nokhodian/mono-agent/internal/nodes/people"
	"github.com/nokhodian/mono-agent/internal/nodes/service"
	"github.com/nokhodian/mono-agent/internal/nodes/system"
	"github.com/nokhodian/mono-agent/internal/workflow"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// reLinkedInActivity matches the numeric activity ID in LinkedIn post URLs.
// e.g. "activity-7123456789" or "activity:7123456789"
var reLinkedInActivity = regexp.MustCompile(`activity[-:](\d+)`)

// isBrowserNodeType returns true for platform.action social/browser node types.
func isBrowserNodeType(t string) bool {
	return strings.HasPrefix(t, "instagram.") || strings.HasPrefix(t, "linkedin.") ||
		strings.HasPrefix(t, "x.") || strings.HasPrefix(t, "tiktok.")
}

// nodeTypeToPlatform maps a node type to its connections-registry platform ID.
// The mapping covers node-type prefixes that don't match their platform ID directly.
var nodeTypeToPlatformOverrides = map[string]string{
	"db.postgres": "postgresql",
	"db.mysql":    "mysql",
	"db.mongodb":  "mongodb",
	"db.redis":    "redis",
}

// nodeTypeToPlatform derives the connections-registry platform ID from a node type.
// Examples:
//
//	"service.google_sheets" → "google_sheets"
//	"service.github"        → "github"
//	"comm.slack"            → "slack"
//	"db.postgres"           → "postgresql"
//	"google_sheets"         → "google_sheets"  (legacy unprefixed)
func nodeTypeToPlatform(nodeType string) string {
	if p, ok := nodeTypeToPlatformOverrides[nodeType]; ok {
		return p
	}
	// Strip known category prefixes.
	for _, prefix := range []string{"service.", "comm.", "db."} {
		if strings.HasPrefix(nodeType, prefix) {
			return strings.TrimPrefix(nodeType, prefix)
		}
	}
	// Already a bare platform name (legacy alias).
	return nodeType
}

// resolveCredentialData looks up a connection by ID or platform name, checks for
// token expiry, and returns the credential data map. This mirrors the Wails app's
// getResourceCredentialData function.
func resolveCredentialData(ctx context.Context, store *connections.Store, credentialOrPlatform string) (map[string]interface{}, error) {
	if store == nil {
		return nil, fmt.Errorf("connections store not available")
	}
	// Try by ID first.
	conn, err := store.Get(ctx, credentialOrPlatform)
	if (err != nil || conn == nil) && credentialOrPlatform != "" {
		// Fallback: look up by platform name.
		conns, lErr := store.ListByPlatform(ctx, credentialOrPlatform)
		if lErr == nil && len(conns) > 0 {
			for i := range conns {
				if conns[i].Status == "active" {
					conn = &conns[i]
					break
				}
			}
			if conn == nil {
				conn = &conns[0]
			}
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("no connection found for %q — run `monoes connect %s` first", credentialOrPlatform, credentialOrPlatform)
	}

	// Check if OAuth token needs refresh (expires within 60 seconds).
	if expiresStr, _ := conn.Data["expires_at"].(string); expiresStr != "" {
		if expiresAt, err := time.Parse(time.RFC3339, expiresStr); err == nil {
			if time.Now().UTC().After(expiresAt.Add(-60 * time.Second)) {
				if refreshed, err := refreshOAuthTokenCLI(ctx, store, conn); err == nil {
					return refreshed, nil
				}
				// If refresh fails, fall through and try with existing (possibly expired) token.
				fmt.Fprintf(os.Stderr, "  Warning: token refresh failed, using existing token\n")
			}
		}
	}

	return conn.Data, nil
}

// refreshOAuthTokenCLI uses the stored refresh_token to obtain a new access_token
// from the provider's token endpoint, updates the connection in the DB, and returns
// the refreshed credential data. This mirrors the Wails app's refreshOAuthToken.
func refreshOAuthTokenCLI(ctx context.Context, store *connections.Store, conn *connections.Connection) (map[string]interface{}, error) {
	refreshToken, _ := conn.Data["refresh_token"].(string)
	if refreshToken == "" {
		return nil, fmt.Errorf("no refresh_token available")
	}

	p, ok := connections.Get(conn.Platform)
	if !ok || p.OAuth == nil {
		return nil, fmt.Errorf("platform %q has no OAuth config", conn.Platform)
	}

	cfg := *p.OAuth
	envPrefix := "MONOES_" + strings.ToUpper(strings.ReplaceAll(p.ID, "-", "_")) + "_"
	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv(envPrefix + "CLIENT_ID")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = os.Getenv(envPrefix + "CLIENT_SECRET")
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("missing OAuth client credentials for refresh (set %sCLIENT_ID)", envPrefix)
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	req, err := http.NewRequest(http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("invalid refresh response: %s", string(body))
	}

	// Update connection data with new tokens.
	conn.Data["access_token"] = tokenResp.AccessToken
	if tokenResp.TokenType != "" {
		conn.Data["token_type"] = tokenResp.TokenType
	}
	if tokenResp.RefreshToken != "" {
		conn.Data["refresh_token"] = tokenResp.RefreshToken
	}
	if tokenResp.ExpiresIn > 0 {
		conn.Data["expires_at"] = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	conn.Status = "active"
	conn.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Persist the refreshed credentials.
	if saveErr := store.Save(ctx, conn); saveErr != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not persist refreshed token: %v\n", saveErr)
	} else {
		fmt.Fprintf(os.Stderr, "  Token refreshed successfully\n")
	}

	return conn.Data, nil
}

// cliSessionProvider launches a headed browser and restores session cookies from the DB.
type cliSessionProvider struct {
	db      *sql.DB
	browser *rod.Browser
}

func (sp *cliSessionProvider) GetPage(ctx context.Context, platform string, username string) (*rod.Page, error) {
	if sp.browser == nil {
		launchURL, err := launcher.New().
			Headless(false).
			Set("disable-blink-features", "AutomationControlled").
			Launch()
		if err != nil {
			return nil, fmt.Errorf("launch browser: %w", err)
		}
		sp.browser = rod.New().ControlURL(launchURL)
		if err := sp.browser.Connect(); err != nil {
			return nil, fmt.Errorf("connect browser: %w", err)
		}
	}

	page, err := sp.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	// Restore session cookies from DB.
	if sp.db != nil {
		var cookiesJSON string
		qErr := sp.db.QueryRow(
			"SELECT cookies_json FROM crawler_sessions WHERE platform = ? ORDER BY expiry DESC LIMIT 1",
			strings.ToLower(platform),
		).Scan(&cookiesJSON)
		if qErr == nil && cookiesJSON != "" {
			var cookies []*proto.NetworkCookieParam
			if json.Unmarshal([]byte(cookiesJSON), &cookies) == nil {
				_ = page.SetCookies(cookies)
			}
		}
	}
	return page, nil
}

func (sp *cliSessionProvider) Close() {
	if sp.browser != nil {
		sp.browser.Close()
	}
}

// cliBotRegistry wraps bot.PlatformRegistry to satisfy nodes.BotRegistry.
type cliBotRegistry struct{}

func (r *cliBotRegistry) GetAdapter(platform string) (action.BotAdapter, bool) {
	factory, ok := bot.PlatformRegistry[strings.ToUpper(platform)]
	if !ok {
		return nil, false
	}
	adapter := factory()
	if ba, ok := adapter.(action.BotAdapter); ok {
		return ba, true
	}
	return nil, false
}

// buildNodeRegistry creates a registry with all built-in node types registered.
// If db is non-nil, AI nodes are also registered (they need an AIStore backed by the DB).
func buildNodeRegistry(verbose bool, db *sql.DB) *workflow.NodeTypeRegistry {
	registry := workflow.NewNodeTypeRegistry()
	control.RegisterAll(registry)
	data.RegisterAll(registry)
	httpnodes.RegisterAll(registry)
	system.RegisterAll(registry)
	dbnodes.RegisterAll(registry)
	comm.RegisterAll(registry)
	service.RegisterAll(registry)
	nodes.RegisterBrowserNodes(registry)
	peoplenodes.RegisterAll(registry, db)

	// Register AI nodes when a database connection is available.
	if db != nil {
		store, err := ai.NewAIStore(db)
		if err == nil {
			ainodes.RegisterAll(registry, store)
		}
	}

	// Register legacy (unprefixed) aliases so old workflows still resolve.
	for legacy, canonical := range map[string]string{
		"google_sheets": "service.google_sheets", "gmail": "service.gmail", "google_drive": "service.google_drive",
		"github": "service.github", "notion": "service.notion", "airtable": "service.airtable",
		"jira": "service.jira", "linear": "service.linear", "asana": "service.asana",
		"stripe": "service.stripe", "shopify": "service.shopify", "salesforce": "service.salesforce",
		"hubspot": "service.hubspot",
		"slack": "comm.slack", "discord": "comm.discord", "telegram": "comm.telegram",
		"twilio": "comm.twilio", "whatsapp": "comm.whatsapp",
		"email_send": "comm.email_send", "email_read": "comm.email_read",
		"mysql": "db.mysql", "postgres": "db.postgres", "mongodb": "db.mongodb", "redis": "db.redis",
		"datetime": "data.datetime", "crypto": "data.crypto", "html": "data.html",
		"xml": "data.xml", "markdown": "data.markdown", "spreadsheet": "data.spreadsheet",
		"compression": "data.compression", "write_binary_file": "data.write_binary_file",
		"if": "core.if", "switch": "core.switch", "merge": "core.merge", "set": "core.set",
		"code": "core.code", "filter": "core.filter", "sort": "core.sort", "limit": "core.limit",
		"aggregate": "core.aggregate", "wait": "core.wait",
		"http_request": "http.request", "http_response": "http.response",
		"execute_command": "system.execute_command", "rss_read": "system.rss_read",
		"read_write_file": "system.read_write_file",
	} {
		registry.Alias(legacy, canonical)
	}

	return registry
}

// newNodeCmd returns the `node` command with subcommands.
func newNodeCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Directly invoke or inspect workflow node types",
	}
	cmd.AddCommand(
		newNodeListCmd(cfg),
		newNodeRunCmd(cfg),
	)
	return cmd
}

// newNodeListCmd lists all registered node types.
func newNodeListCmd(cfg *globalConfig) *cobra.Command {
	var filter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available node types",
		Example: `  monoes node list
  monoes node list --filter comm
  monoes node list --filter http`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Open DB on best-effort basis so AI nodes appear in the list.
			var rawDB *sql.DB
			if db, err := initDB(cfg); err == nil {
				rawDB = db.DB
				defer db.Close()
			}
			registry := buildNodeRegistry(cfg.Verbose, rawDB)
			types := registry.Types()
			sort.Strings(types)

			if filter != "" {
				var filtered []string
				for _, t := range types {
					if strings.Contains(t, filter) {
						filtered = append(filtered, t)
					}
				}
				types = filtered
			}

			if cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(types)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "TYPE\tCATEGORY")
			fmt.Fprintln(w, "────\t────────")
			for _, t := range types {
				cat := nodeCategory(t)
				fmt.Fprintf(w, "%s\t%s\n", t, cat)
			}
			w.Flush()
			fmt.Printf("\n%d node types\n", len(types))
			return nil
		},
	}

	cmd.Flags().StringVar(&filter, "filter", "", "Filter node types by substring")
	return cmd
}

// newNodeRunCmd runs a single node type with provided config and input items.
func newNodeRunCmd(cfg *globalConfig) *cobra.Command {
	var (
		configJSON   string
		inputJSON    string
		outputFmt    string
		credentialID string
	)

	cmd := &cobra.Command{
		Use:   "run <node-type>",
		Short: "Execute a node type directly with given config and input",
		Long: `Execute any registered node type as a standalone operation.
Config and input items are passed as JSON. Results are printed to stdout.

Node types follow the pattern: category.name (e.g. http.request, comm.slack, control.if)
Browser/social nodes require --config to include "username" and a session must exist.

Credentials are resolved automatically from stored connections when credential_id
is not provided in config. You can also pass --credential with a connection ID or
platform name to override. Token refresh is handled automatically for OAuth connections.`,
		Example: `  # HTTP GET request
  monoes node run http.request --config '{"method":"GET","url":"https://httpbin.org/get"}'

  # Hash a value with crypto node
  monoes node run crypto --config '{"operation":"hash","algorithm":"sha256","value":"hello world"}'

  # Send a Slack message
  monoes node run comm.slack --config '{"token":"xoxb-...","operation":"post_message","channel":"#general","message":"hello"}'

  # Run with input items from JSON
  monoes node run control.set \
    --config '{"fields":{"status":"done"}}' \
    --input '[{"json":{"id":1,"name":"Alice"}}]'

  # Filter items
  monoes node run control.filter \
    --config '{"condition":"{{eq $json.active true}}"}' \
    --input '[{"json":{"id":1,"active":true}},{"json":{"id":2,"active":false}}]'

  # Sort items
  monoes node run control.sort \
    --config '{"key":"name","order":"asc"}' \
    --input '[{"json":{"name":"Charlie"}},{"json":{"name":"Alice"}},{"json":{"name":"Bob"}}]'

  # Execute a shell command
  monoes node run system.execute_command --config '{"command":"echo hello world"}'

  # Read an RSS feed
  monoes node run system.rss_read --config '{"url":"https://hnrss.org/frontpage","limit":5}'

  # Parse HTML
  monoes node run html --config '{"operation":"extract","selector":"h1","attribute":"text"}' \
    --input '[{"json":{"html":"<h1>Hello</h1><h1>World</h1>"}}]'

  # MySQL query (requires running DB)
  monoes node run mysql --config '{"dsn":"user:pass@tcp(localhost:3306)/db","operation":"query","query":"SELECT 1 AS n"}'

  # Aggregate items
  monoes node run control.aggregate \
    --config '{"operation":"sum","field":"amount"}' \
    --input '[{"json":{"amount":10}},{"json":{"amount":20}},{"json":{"amount":30}}]'

  # Google Sheets (auto-resolves credential from stored connections)
  monoes node run service.google_sheets --config '{"operation":"read_rows","spreadsheetId":"abc123","range":"Sheet1!A1:D10"}'

  # Explicit credential by connection ID or platform name
  monoes node run service.google_sheets --credential google_sheets --config '{"operation":"read_rows","spreadsheetId":"abc123"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeType := args[0]

			// Open DB so AI nodes are available for execution.
			var rawDB *sql.DB
			if db, err := initDB(cfg); err == nil {
				rawDB = db.DB
				defer db.Close()
			}
			registry := buildNodeRegistry(cfg.Verbose, rawDB)
			factory, ok := registry.Get(nodeType)
			if !ok {
				// Show close matches
				all := registry.Types()
				sort.Strings(all)
				var matches []string
				for _, t := range all {
					if strings.Contains(t, nodeType) || strings.Contains(nodeType, t) {
						matches = append(matches, t)
					}
				}
				if len(matches) > 0 {
					return fmt.Errorf("unknown node type %q. Did you mean one of: %s", nodeType, strings.Join(matches, ", "))
				}
				return fmt.Errorf("unknown node type %q. Run `monoes node list` to see all types", nodeType)
			}

			// Parse config
			config := map[string]interface{}{}
			if configJSON != "" {
				if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
					return fmt.Errorf("invalid --config JSON: %w", err)
				}
			}

			// Resolve credentials: --credential flag → config.credential_id → auto-resolve by platform.
			// This matches the Wails app's getResourceCredentialData pattern.
			if rawDB != nil {
				connStore := connections.NewStore(rawDB)
				credKey := credentialID // from --credential flag
				if credKey == "" {
					if cid, ok := config["credential_id"].(string); ok && cid != "" {
						credKey = cid
					}
				}
				// Auto-resolve: if still empty, derive platform from node type and look up.
				if credKey == "" {
					credKey = nodeTypeToPlatform(nodeType)
				}
				if credKey != "" {
					credData, err := resolveCredentialData(context.Background(), connStore, credKey)
					if err == nil && credData != nil {
						// Merge credential data into config (access_token, refresh_token, etc.).
						for k, v := range credData {
							if _, exists := config[k]; !exists {
								config[k] = v
							}
						}
						config["credential"] = credData
						if cfg.Verbose {
							fmt.Fprintf(os.Stderr, "  Resolved credential for platform %q\n", credKey)
						}
					} else if credentialID != "" {
						// Only error if --credential was explicitly provided.
						return fmt.Errorf("credential resolution failed: %w", err)
					}
					// If auto-resolve fails silently, the node may still work
					// with credentials passed directly in config (e.g., --config '{"token":"..."}').
				}
			}

			// Parse input items
			var inputItems []workflow.Item
			if inputJSON != "" {
				if err := json.Unmarshal([]byte(inputJSON), &inputItems); err != nil {
					// Also try parsing a single object as a one-item array
					var single map[string]interface{}
					if err2 := json.Unmarshal([]byte(inputJSON), &single); err2 != nil {
						return fmt.Errorf("invalid --input JSON (expected array of items or single object): %w", err)
					}
					inputItems = []workflow.Item{{JSON: single}}
				}
			}
			// Default: one empty item (most nodes require at least one input item)
			if len(inputItems) == 0 {
				inputItems = []workflow.Item{{JSON: map[string]interface{}{}}}
			}

			logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
			if !cfg.Verbose {
				logger = logger.Level(zerolog.WarnLevel)
			}
			_ = logger

			// Set up browser session provider, bot registry, and config manager for social/browser nodes.
			if isBrowserNodeType(nodeType) {
				sp := &cliSessionProvider{db: rawDB}
				defer sp.Close()
				nodes.SetGlobalSessionProvider(sp)
				nodes.SetGlobalBotRegistry(&cliBotRegistry{})
				nodes.SetGlobalCredentialStore(connections.NewStore(rawDB))

				// Wire up config manager for selector resolution.
				cfgLogger := zerolog.New(os.Stderr).Level(zerolog.WarnLevel)
				var cfgStore cfgpkg.ConfigStore
				if cfgDB, err := initDB(cfg); err == nil {
					cfgStore = &cfgpkg.DBConfigStore{DB: cfgDB}
					defer cfgDB.Close()
				}
				apiClient := cfgpkg.NewAPIClient(cfgLogger)
				rawCfgMgr := cfgpkg.NewConfigManager(expandPath("~/.monoes/configs"), cfgStore, apiClient, cfgLogger)
				nodes.SetGlobalConfigMgr(&cfgpkg.ConfigManagerAdapter{Mgr: rawCfgMgr})
			}

			executor := factory()
			input := workflow.NodeInput{
				Items:       inputItems,
				NodeOutputs: map[string][]workflow.Item{},
				WorkflowID:  "cli",
				ExecutionID: "cli",
				NodeID:      "cli-node",
				NodeName:    nodeType,
			}

			ctx := context.Background()
			outputs, err := executor.Execute(ctx, input, config)
			if err != nil {
				return fmt.Errorf("node %s failed: %w", nodeType, err)
			}

			// Auto-save to people table for profile-scraping nodes.
			if strings.HasSuffix(nodeType, "scrape_profile_info") && rawDB != nil {
				var allItems []workflow.Item
				for _, o := range outputs {
					allItems = append(allItems, o.Items...)
				}
				if len(allItems) > 0 {
					peopleSaver := &peoplenodes.PeopleSaveNode{}
					saveInput := workflow.NodeInput{Items: allItems}
					_, saveErr := peopleSaver.Execute(ctx, saveInput, config)
					if saveErr != nil {
						fmt.Fprintf(os.Stderr, "  Warning: failed to save profiles to people table: %v\n", saveErr)
					} else {
						fmt.Fprintf(os.Stderr, "  Saved %d profile(s) to people table\n", len(allItems))
					}
				}
			}

			// Auto-save posts to posts table after list_user_posts.
			if strings.HasSuffix(nodeType, "list_user_posts") && rawDB != nil {
				var allItems []workflow.Item
				for _, o := range outputs {
					allItems = append(allItems, o.Items...)
				}
				if len(allItems) > 0 {
					saved, skipped, failed := savePostsToDB(ctx, rawDB, allItems, nodeType, config)
					fmt.Fprintf(os.Stderr, "  Saved %d post(s) to posts table (%d skipped, %d failed)\n", saved, skipped, failed)
				}
			}

			// Auto-save comments to post_comments table after list_post_comments.
			if strings.HasSuffix(nodeType, "list_post_comments") && rawDB != nil {
				var allItems []workflow.Item
				for _, o := range outputs {
					allItems = append(allItems, o.Items...)
				}
				if len(allItems) > 0 {
					// Resolve post_id from config selectedListItems[0] (the post URL input declared in the action JSON).
					postID := ""
					platform := strings.ToUpper(strings.SplitN(nodeType, ".", 2)[0])
					postURL := ""
					if items, ok := config["selectedListItems"].([]interface{}); ok && len(items) > 0 {
						switch v := items[0].(type) {
						case string:
							postURL = v
						case map[string]interface{}:
							postURL, _ = v["url"].(string)
						}
					}
					if postURL != "" {
						shortcode := extractPostShortcode(postURL)
						if shortcode != "" {
							_ = rawDB.QueryRowContext(ctx,
								"SELECT id FROM posts WHERE platform = ? AND shortcode = ?",
								platform, shortcode,
							).Scan(&postID)
						}
					}
					if postID == "" {
						fmt.Fprintf(os.Stderr, "  Warning: post not found in DB — run list_user_posts first\n")
					} else {
						saved, skipped, failed := saveCommentsToDB(ctx, rawDB, allItems, postID)
						fmt.Fprintf(os.Stderr, "  Saved %d comment(s) to post_comments table (%d skipped, %d failed)\n", saved, skipped, failed)
					}
				}
			}

			// Render output
			switch outputFmt {
			case "json":
				result := map[string]interface{}{}
				for _, o := range outputs {
					result[o.Handle] = o.Items
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)

			case "jsonl":
				for _, o := range outputs {
					for _, item := range o.Items {
						b, _ := json.Marshal(map[string]interface{}{
							"handle": o.Handle,
							"json":   item.JSON,
						})
						fmt.Println(string(b))
					}
				}
				return nil

			default: // table / pretty
				for _, o := range outputs {
					if len(outputs) > 1 {
						fmt.Printf("\n── handle: %s (%d items) ──\n", o.Handle, len(o.Items))
					}
					for i, item := range o.Items {
						if len(o.Items) > 1 {
							fmt.Printf("  [%d] ", i)
						} else {
							fmt.Print("  ")
						}
						b, _ := json.MarshalIndent(item.JSON, "  ", "  ")
						fmt.Println(string(b))
					}
				}
				totalItems := 0
				for _, o := range outputs {
					totalItems += len(o.Items)
				}
				fmt.Printf("\n✓  %d output handle(s), %d total item(s)\n", len(outputs), totalItems)
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&configJSON, "config", "", "Node config as JSON object")
	cmd.Flags().StringVar(&inputJSON, "input", "", "Input items as JSON array of {\"json\":{...}} objects, or a single JSON object")
	cmd.Flags().StringVar(&outputFmt, "output", "pretty", "Output format: pretty|json|jsonl")
	cmd.Flags().StringVar(&credentialID, "credential", "", "Connection ID or platform name for credential lookup (auto-resolved from node type if omitted)")
	return cmd
}

// savePostsToDB upserts scraped post items into the posts table.
// Returns (saved, skipped, failed) counts.
func savePostsToDB(ctx context.Context, db *sql.DB, items []workflow.Item, nodeType string, config map[string]interface{}) (int, int, int) {
	// Derive platform from nodeType prefix e.g. "instagram.list_user_posts" → "INSTAGRAM"
	platform := strings.ToUpper(strings.SplitN(nodeType, ".", 2)[0])

	// Resolve person_id: find username from config targets, look up people table.
	personID := ""
	if targets, ok := config["targets"].([]interface{}); ok && len(targets) > 0 {
		if t, ok := targets[0].(map[string]interface{}); ok {
			postURL, _ := t["url"].(string)
			username := ""
			if factory, ok := bot.PlatformRegistry[platform]; ok {
				username = factory().ExtractUsername(postURL)
			}
			if username != "" {
				_ = db.QueryRowContext(ctx,
					"SELECT id FROM people WHERE platform_username = ? AND UPPER(platform) = ?",
					username, platform,
				).Scan(&personID)
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	saved, skipped, failed := 0, 0, 0

	for _, item := range items {
		data := item.JSON
		shortcode, _ := data["shortcode"].(string)
		postURL, _ := data["url"].(string)

		// Fallback: extract shortcode from URL if not present as a field.
		if shortcode == "" && postURL != "" {
			shortcode = extractPostShortcode(postURL)
		}
		if shortcode == "" {
			skipped++
			continue
		}
		if postURL == "" {
			skipped++
			continue
		}

		thumbnail, _ := data["thumbnail_src"].(string)
		caption, _ := data["alt_text"].(string)

		var personIDArg interface{}
		if personID != "" {
			personIDArg = personID
		}

		_, err := db.ExecContext(ctx,
			`INSERT INTO posts (id, person_id, platform, shortcode, url, thumbnail_url, like_count, comment_count, caption, scraped_at)
             VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)
             ON CONFLICT(platform, shortcode)
             DO UPDATE SET
               thumbnail_url = COALESCE(excluded.thumbnail_url, posts.thumbnail_url),
               caption       = COALESCE(excluded.caption,       posts.caption),
               person_id     = COALESCE(excluded.person_id,     posts.person_id),
               scraped_at    = excluded.scraped_at`,
			uuid.New().String(), personIDArg, platform, shortcode, postURL,
			nullableStrCLI(thumbnail), nullableStrCLI(caption), now,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to save post %s: %v\n", shortcode, err)
			failed++
		} else {
			saved++
		}
	}
	return saved, skipped, failed
}

// extractPostShortcode extracts the platform shortcode from a post URL.
// Instagram: https://www.instagram.com/p/CD61bhxKOQh/ → "CD61bhxKOQh"
// LinkedIn:  https://www.linkedin.com/posts/user-activity-7123456789/ → "7123456789"
func extractPostShortcode(postURL string) string {
	// Instagram: /p/{shortcode}/ or /reel/{shortcode}/
	parts := strings.Split(strings.Trim(postURL, "/"), "/")
	for i, p := range parts {
		if (p == "p" || p == "reel") && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	// LinkedIn: activity-NNNNNNNN (posts URL) or activity:NNNNNNNN (feed/update URL)
	if strings.Contains(postURL, "linkedin.com") {
		if m := reLinkedInActivity.FindStringSubmatch(postURL); len(m) > 1 {
			return m[1]
		}
	}

	return ""
}

func nullableStrCLI(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// saveCommentsToDB upserts scraped comment items into the post_comments table.
// Returns (saved, skipped, failed) counts.
func saveCommentsToDB(ctx context.Context, db *sql.DB, items []workflow.Item, postID string) (int, int, int) {
	now := time.Now().UTC().Format(time.RFC3339)
	saved, skipped, failed := 0, 0, 0

	for _, item := range items {
		data := item.JSON
		author, _ := data["author"].(string)
		if author == "" {
			skipped++
			continue
		}
		text, _ := data["text"].(string)
		timestamp, _ := data["timestamp"].(string)
		// Leave timestamp as "" if not provided — this is the stable dedup key.
		// Do NOT substitute current time here, as that defeats UNIQUE(post_id, author, timestamp).

		likesCount := int64(0)
		switch v := data["likes_count"].(type) {
		case float64:
			likesCount = int64(v)
		case int64:
			likesCount = v
		}
		replyCount := int64(0)
		switch v := data["reply_count"].(type) {
		case float64:
			replyCount = int64(v)
		case int64:
			replyCount = v
		}

		_, err := db.ExecContext(ctx,
			`INSERT INTO post_comments (id, post_id, author, text, timestamp, likes_count, reply_count, scraped_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?)
             ON CONFLICT(post_id, author, timestamp)
             DO UPDATE SET
               text        = COALESCE(excluded.text,        post_comments.text),
               likes_count = excluded.likes_count,
               reply_count = excluded.reply_count,
               scraped_at  = excluded.scraped_at`,
			uuid.New().String(), postID, author,
			nullableStrCLI(text), timestamp, likesCount, replyCount, now,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to save comment by %s: %v\n", author, err)
			failed++
		} else {
			saved++
		}
	}
	return saved, skipped, failed
}

// nodeCategory infers a display category from a node type string.
func nodeCategory(t string) string {
	switch {
	case strings.HasPrefix(t, "trigger."):
		return "trigger"
	case strings.HasPrefix(t, "control."), t == "if" || t == "switch" || t == "merge" || t == "set" || t == "code" || t == "filter" || t == "sort" || t == "limit" || t == "aggregate" || t == "wait":
		return "control"
	case strings.HasPrefix(t, "http."):
		return "http"
	case strings.HasPrefix(t, "system."):
		return "system"
	case strings.HasPrefix(t, "comm."):
		return "communication"
	case strings.HasPrefix(t, "ai."):
		return "ai"
	case strings.HasPrefix(t, "instagram."), strings.HasPrefix(t, "linkedin."), strings.HasPrefix(t, "x."), strings.HasPrefix(t, "tiktok."):
		return "browser/social"
	case strings.HasPrefix(t, "people."):
		return "people"
	case t == "mysql" || t == "postgres" || t == "mongodb" || t == "redis":
		return "database"
	case t == "github" || t == "notion" || t == "airtable" || t == "jira" || t == "linear" || t == "asana" || t == "stripe" || t == "shopify" || t == "salesforce" || t == "hubspot" || t == "google_sheets" || t == "gmail" || t == "google_drive":
		return "service"
	case t == "datetime" || t == "crypto" || t == "html" || t == "xml" || t == "markdown" || t == "spreadsheet" || t == "compression" || t == "write_binary_file":
		return "data"
	default:
		return "other"
	}
}
