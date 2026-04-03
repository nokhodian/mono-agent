//go:build integration

package action

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"

	// Import Instagram bot to trigger init() registration.
	"github.com/nokhodian/mono-agent/internal/bot"
	_ "github.com/nokhodian/mono-agent/internal/bot/instagram"

	// Import SQLite driver for session cookie loading.
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testStorageAdapter is a minimal StorageInterface for testing.
type testStorageAdapter struct {
	stateUpdates   []string
	reachedIndexes []int
	extractedData  [][]map[string]interface{}
}

func (s *testStorageAdapter) UpdateActionState(id, state string) error {
	s.stateUpdates = append(s.stateUpdates, state)
	return nil
}

func (s *testStorageAdapter) UpdateActionReachedIndex(id string, index int) error {
	s.reachedIndexes = append(s.reachedIndexes, index)
	return nil
}

func (s *testStorageAdapter) SaveExtractedData(actionID string, items []map[string]interface{}) error {
	s.extractedData = append(s.extractedData, items)
	return nil
}

// launchTestBrowser creates a visible Chrome browser with Instagram session
// cookies restored. Requires a valid session in ~/.monoes/monoes.db.
func launchTestBrowser(t *testing.T) (*rod.Browser, *rod.Page, func()) {
	t.Helper()

	launchURL, err := launcher.New().
		Headless(false).
		Set("disable-blink-features", "AutomationControlled").
		Launch()
	if err != nil {
		t.Fatalf("Failed to launch browser: %v", err)
	}

	browser := rod.New().ControlURL(launchURL)
	if err := browser.Connect(); err != nil {
		t.Fatalf("Failed to connect to browser: %v", err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		browser.Close()
		t.Fatalf("Failed to create page: %v", err)
	}

	// Restore session cookies from the database.
	dbPath := os.ExpandEnv("$HOME/.monoes/monoes.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		browser.Close()
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	var cookiesJSON string
	err = db.QueryRow(
		"SELECT cookies_json FROM crawler_sessions WHERE platform = 'instagram' ORDER BY expiry DESC LIMIT 1",
	).Scan(&cookiesJSON)
	if err != nil {
		browser.Close()
		t.Fatalf("No Instagram session found in DB. Run `monoes login instagram` first. Error: %v", err)
	}

	if cookiesJSON != "" {
		var cookies []*proto.NetworkCookieParam
		if jsonErr := json.Unmarshal([]byte(cookiesJSON), &cookies); jsonErr != nil {
			t.Logf("Warning: could not parse cookies: %v", jsonErr)
		} else {
			if setErr := page.SetCookies(cookies); setErr != nil {
				t.Logf("Warning: could not set cookies: %v", setErr)
			} else {
				t.Log("Instagram session cookies restored successfully")
			}
		}
	}

	cleanup := func() {
		browser.Close()
	}

	return browser, page, cleanup
}

// newTestLogger creates a zerolog logger that writes to test output.
func newTestLogger() zerolog.Logger {
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).
		With().Timestamp().Logger()
}

// newTestExecutor creates an ActionExecutor wired for testing.
func newTestExecutor(
	t *testing.T,
	ctx context.Context,
	page *rod.Page,
	logger zerolog.Logger,
) (*ActionExecutor, *testStorageAdapter, chan ExecutionEvent) {
	t.Helper()

	sa := &testStorageAdapter{}

	// Get Instagram bot adapter for call_bot_method steps.
	var botAdapter BotAdapter
	if factory, ok := bot.PlatformRegistry["INSTAGRAM"]; ok {
		adapter := factory()
		if ba, ok := adapter.(BotAdapter); ok {
			botAdapter = ba
		}
	}

	events := make(chan ExecutionEvent, 100)

	executor := NewActionExecutor(
		ctx,
		page,
		sa,
		nil, // configMgr — tests use hardcoded XPaths from action definitions
		events,
		botAdapter,
		logger,
	)

	return executor, sa, events
}

// drainEvents reads and logs all events from the channel until it's closed.
func drainEvents(t *testing.T, events chan ExecutionEvent) {
	t.Helper()
	for evt := range events {
		t.Logf("  [event] %s: %s", evt.Type, evt.Message)
	}
}

// printReport logs a structured test report for an action execution.
func printReport(t *testing.T, testName string, result *ExecutionResult, sa *testStorageAdapter, err error) {
	t.Helper()
	t.Logf("")
	t.Logf("╔══════════════════════════════════════════════╗")
	t.Logf("║  %s TEST REPORT", testName)
	t.Logf("╠══════════════════════════════════════════════╣")

	if err != nil {
		t.Logf("║  Execution error: %v", err)
	} else {
		t.Logf("║  Execution: SUCCESS (no error)")
	}

	t.Logf("║  State transitions: %v", sa.stateUpdates)
	t.Logf("║  Reached index updates: %v", sa.reachedIndexes)

	if result != nil {
		t.Logf("║  Total processed: %d", result.TotalProcessed)
		t.Logf("║  Extracted items: %d", len(result.ExtractedItems))
		t.Logf("║  Failed items: %d", len(result.FailedItems))
		t.Logf("║  Duration: %s", result.Duration.Truncate(time.Millisecond))

		if len(result.FailedItems) > 0 {
			t.Logf("║  --- Failed steps ---")
			for _, fi := range result.FailedItems {
				t.Logf("║    Step %q (index %d): %v", fi.StepID, fi.Index, fi.Error)
			}
		}

		if len(result.ExtractedItems) > 0 {
			t.Logf("║  --- Extracted data ---")
			for i, item := range result.ExtractedItems {
				data, _ := json.Marshal(item)
				t.Logf("║    [%d] %s", i, string(data))
			}
		}
	} else {
		t.Logf("║  Result: nil")
	}

	t.Logf("╚══════════════════════════════════════════════╝")
}

// ---------------------------------------------------------------------------
// Test: comment_on_posts
// ---------------------------------------------------------------------------

func TestInstagramPostCommenting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: comment on mortezanoes' post.
	postURL := "https://www.instagram.com/p/CFEtpoYKB53/"
	commentText := "hello"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      postURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})
	executor.SetVariable("commentText", commentText)

	action := &StorageAction{
		ID:             "test-comment-001",
		Type:           "comment_on_posts",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Post Commenting",
	}

	t.Logf("Starting comment_on_posts test...")
	t.Logf("  Post URL: %s", postURL)
	t.Logf("  Comment text: %q", commentText)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "comment_on_posts", result, sa, execErr)

	// Assertions.
	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
	// The action should have transitioned to RUNNING.
	if len(sa.stateUpdates) == 0 || sa.stateUpdates[0] != "RUNNING" {
		t.Errorf("Expected state to transition to RUNNING, got: %v", sa.stateUpdates)
	}
}

// ---------------------------------------------------------------------------
// Test: send_dms
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Test: like_posts
// ---------------------------------------------------------------------------

func TestInstagramPostLiking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: like a post on mortezanoes' account.
	postURL := "https://www.instagram.com/p/CFEtpoYKB53/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      postURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})

	action := &StorageAction{
		ID:             "test-like-001",
		Type:           "like_posts",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Post Liking",
	}

	t.Logf("Starting like_posts test...")
	t.Logf("  Post URL: %s", postURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "like_posts", result, sa, execErr)

	// Assertions.
	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
	if len(sa.stateUpdates) == 0 || sa.stateUpdates[0] != "RUNNING" {
		t.Errorf("Expected state to transition to RUNNING, got: %v", sa.stateUpdates)
	}
}

// ---------------------------------------------------------------------------
// Test: send_dms
// ---------------------------------------------------------------------------

func TestInstagramBulkMessaging(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: send a DM to mortezanoes (user's own account).
	profileURL := "https://www.instagram.com/mortezanoes/"
	messageText := "hello"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      profileURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})
	executor.SetVariable("messageText", messageText)

	action := &StorageAction{
		ID:             "test-message-001",
		Type:           "send_dms",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Bulk Messaging",
	}

	t.Logf("Starting send_dms test...")
	t.Logf("  Profile URL: %s", profileURL)
	t.Logf("  Message text: %q", messageText)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "send_dms", result, sa, execErr)

	// Assertions.
	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
	if len(sa.stateUpdates) == 0 || sa.stateUpdates[0] != "RUNNING" {
		t.Errorf("Expected state to transition to RUNNING, got: %v", sa.stateUpdates)
	}
}

// ---------------------------------------------------------------------------
// Test: comment_on_posts (via comment_post bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramCommentPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: comment on mortezanoes' post.
	postURL := "https://www.instagram.com/p/CFEtpoYKB53/"
	commentText := "hello"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      postURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})
	executor.SetVariable("commentText", commentText)

	action := &StorageAction{
		ID:             "test-comment-bot-001",
		Type:           "comment_on_posts",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Comment Post (3-Tier)",
	}

	t.Logf("Starting comment_on_posts test (3-tier)...")
	t.Logf("  Post URL: %s", postURL)
	t.Logf("  Comment text: %q", commentText)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "comment_on_posts (3-Tier)", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Test: export_followers (via fetch_followers_list bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramFetchFollowers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: fetch followers from mortezanoes' profile.
	profileURL := "https://www.instagram.com/mortezanoes/"

	executor.SetVariable("profileUrl", profileURL)
	executor.SetVariable("sourceType", "FOLLOWERS_FETCH")
	executor.SetVariable("maxResultsCount", 10)

	action := &StorageAction{
		ID:             "test-fetch-followers-001",
		Type:           "export_followers",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Fetch Followers",
	}

	t.Logf("Starting export_followers test...")
	t.Logf("  Profile URL: %s", profileURL)
	t.Logf("  Source type: FOLLOWERS_FETCH")
	t.Logf("  Max count: 10")

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "export_followers", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
}

// ---------------------------------------------------------------------------
// Test: publish_post (via publish_content bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramPublishContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: publish a test post.
	// NOTE: Replace with a valid local image path before running.
	mediaPath := "/tmp/test_image.jpg"
	caption := fmt.Sprintf("Automated test post %d", time.Now().Unix())

	executor.SetVariable("media", []map[string]interface{}{
		{"url": mediaPath},
	})
	executor.SetVariable("text", caption)
	executor.SetVariable("locationTag", "")

	action := &StorageAction{
		ID:             "test-publish-001",
		Type:           "publish_post",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Publish Content",
	}

	t.Logf("Starting publish_post test...")
	t.Logf("  Media path: %s", mediaPath)
	t.Logf("  Caption: %q", caption)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "publish_post", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
}

// ---------------------------------------------------------------------------
// Test: engage_with_posts (via interact_with_posts bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramInteractWithPosts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: interact with posts tagged "test".
	keyword := "test"
	maxContentCount := 2

	executor.SetVariable("searches", []map[string]interface{}{
		{"keyword": keyword},
	})
	executor.SetVariable("maxContentCount", maxContentCount)
	executor.SetVariable("commentText", "")
	executor.SetVariable("target", "")

	action := &StorageAction{
		ID:             "test-interact-posts-001",
		Type:           "engage_with_posts",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Interact With Posts",
	}

	t.Logf("Starting engage_with_posts test...")
	t.Logf("  Keyword: %s", keyword)
	t.Logf("  Max content count: %d", maxContentCount)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "engage_with_posts", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
}

// ---------------------------------------------------------------------------
// Test: auto_reply_dms (via reply_to_conversation bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramBulkReplying(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: reply to a conversation in DM inbox.
	// Use a known conversation URL (a DM thread with yourself or a test account).
	conversationURL := "https://www.instagram.com/direct/t/340282366841710300949128137443944319876/"
	replyText := "hello"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      conversationURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})
	executor.SetVariable("replyText", replyText)
	executor.SetVariable("startDate", "2020-01-01")
	executor.SetVariable("endDate", "2030-12-31")
	executor.SetVariable("pollInterval", 5)

	action := &StorageAction{
		ID:             "test-reply-001",
		Type:           "auto_reply_dms",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Bulk Replying",
	}

	t.Logf("Starting auto_reply_dms test...")
	t.Logf("  Conversation URL: %s", conversationURL)
	t.Logf("  Reply text: %q", replyText)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "auto_reply_dms", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Test: scrape_profile_info (via get_user_info bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramProfileSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: search profile info for mortezanoes.
	profileURL := "https://www.instagram.com/mortezanoes/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      profileURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})

	action := &StorageAction{
		ID:             "test-profile-search-001",
		Type:           "scrape_profile_info",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Profile Search",
	}

	t.Logf("Starting scrape_profile_info test...")
	t.Logf("  Profile URL: %s", profileURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "scrape_profile_info", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Test: find_by_keyword
// ---------------------------------------------------------------------------

func TestInstagramKeywordSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: search for posts tagged "travel" and extract profile info.
	keyword := "travel"
	maxResultsCount := 3

	executor.SetVariable("keyword", keyword)
	executor.SetVariable("maxResultsCount", maxResultsCount)

	action := &StorageAction{
		ID:             "test-keyword-search-001",
		Type:           "find_by_keyword",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Keyword Search",
	}

	t.Logf("Starting find_by_keyword test...")
	t.Logf("  Keyword: %s", keyword)
	t.Logf("  Max results: %d", maxResultsCount)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "find_by_keyword", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
}

// ---------------------------------------------------------------------------
// Test: engage_user_posts (via interact_with_user_posts bot method)
// ---------------------------------------------------------------------------

func TestInstagramUserPostsInteraction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: interact with posts from mortezanoes' profile.
	username := "mortezanoes"
	maxContentCount := 2

	executor.SetVariable("username", username)
	executor.SetVariable("maxContentCount", maxContentCount)
	executor.SetVariable("commentText", "")

	action := &StorageAction{
		ID:             "test-user-posts-001",
		Type:           "engage_user_posts",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test User Posts Interaction",
	}

	t.Logf("Starting engage_user_posts test...")
	t.Logf("  Username: %s", username)
	t.Logf("  Max content count: %d", maxContentCount)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "engage_user_posts", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
}

// ---------------------------------------------------------------------------
// Test: follow_users (via follow_user bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramBulkFollowing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: follow mortezanoes profile.
	profileURL := "https://www.instagram.com/mortezanoes/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      profileURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})

	action := &StorageAction{
		ID:             "test-follow-001",
		Type:           "follow_users",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Bulk Following",
	}

	t.Logf("Starting follow_users test...")
	t.Logf("  Profile URL: %s", profileURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "follow_users", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Test: unfollow_users (via unfollow_user bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramBulkUnfollowing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: unfollow mortezanoes profile.
	profileURL := "https://www.instagram.com/mortezanoes/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      profileURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})

	action := &StorageAction{
		ID:             "test-unfollow-001",
		Type:           "unfollow_users",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Bulk Unfollowing",
	}

	t.Logf("Starting unfollow_users test...")
	t.Logf("  Profile URL: %s", profileURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "unfollow_users", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Test: watch_stories (via view_stories bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramStoryViewing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: view stories on a profile that likely has active stories.
	// Using instagram's own account as it typically has stories.
	profileURL := "https://www.instagram.com/instagram/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      profileURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})

	action := &StorageAction{
		ID:             "test-story-001",
		Type:           "watch_stories",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Story Viewing",
	}

	t.Logf("Starting watch_stories test...")
	t.Logf("  Profile URL: %s", profileURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "watch_stories", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
}

// ---------------------------------------------------------------------------
// Test: extract_post_data (via scrape_post_data bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramPostScraping(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: scrape data from mortezanoes' post.
	postURL := "https://www.instagram.com/p/CFEtpoYKB53/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      postURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})

	action := &StorageAction{
		ID:             "test-scrape-001",
		Type:           "extract_post_data",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Post Scraping",
	}

	t.Logf("Starting extract_post_data test...")
	t.Logf("  Post URL: %s", postURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "extract_post_data", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	// Scraping should produce extracted items.
	if len(result.ExtractedItems) > 0 {
		t.Logf("Scraped %d items successfully", len(result.ExtractedItems))
	}
}

// ---------------------------------------------------------------------------
// Test: like_comments_on_posts (via like_comment bot method — Tier 1)
// ---------------------------------------------------------------------------

func TestInstagramCommentLiking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := newTestLogger()
	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	executor, sa, events := newTestExecutor(t, ctx, page, logger)
	go drainEvents(t, events)

	// Test data: like a comment on mortezanoes' post.
	postURL := "https://www.instagram.com/p/CFEtpoYKB53/"

	executor.SetVariable("selectedListItems", []map[string]interface{}{
		{
			"url":      postURL,
			"platform": "instagram",
			"status":   "PENDING",
		},
	})
	// Empty commentAuthor will fall back to liking the most recent comment.
	executor.SetVariable("commentAuthor", "")

	action := &StorageAction{
		ID:             "test-comment-like-001",
		Type:           "like_comments_on_posts",
		TargetPlatform: "instagram",
		State:          "PENDING",
		Title:          "Test Comment Liking",
	}

	t.Logf("Starting like_comments_on_posts test...")
	t.Logf("  Post URL: %s", postURL)

	result, execErr := executor.Execute(action)
	close(events)

	printReport(t, "like_comments_on_posts", result, sa, execErr)

	if result == nil {
		t.Fatal("FAIL: Expected non-nil execution result")
	}
	if len(result.FailedItems) > 0 {
		t.Errorf("FAIL: %d step(s) failed. First failure: step=%s err=%v",
			len(result.FailedItems), result.FailedItems[0].StepID, result.FailedItems[0].Error)
	}
}
