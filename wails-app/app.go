package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/google/uuid"
	"github.com/nokhodian/mono-agent/internal/bot"
	_ "github.com/nokhodian/mono-agent/internal/bot/instagram"
	_ "github.com/nokhodian/mono-agent/internal/bot/linkedin"
	_ "github.com/nokhodian/mono-agent/internal/bot/tiktok"
	_ "github.com/nokhodian/mono-agent/internal/bot/x"
	"github.com/nokhodian/mono-agent/internal/action"
	"github.com/nokhodian/mono-agent/internal/ai"
	aichat "github.com/nokhodian/mono-agent/internal/ai/chat"
	"github.com/nokhodian/mono-agent/internal/config"
	"github.com/nokhodian/mono-agent/internal/connections"
	"github.com/nokhodian/mono-agent/internal/workflow"
	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	_ "modernc.org/sqlite"
)

// App holds application state bound to the Wails runtime.
type App struct {
	ctx     context.Context
	db      *sql.DB
	dbPath  string
	logs    []LogEntry
	connMgr     *connections.Manager
	aiStore     *ai.AIStore
	chatService *aichat.ChatService
	cfgMgr      action.ConfigInterface
	wfStore     *workflow.HybridWorkflowStore
}

// NewApp creates the App instance.
func NewApp() *App {
	home, _ := os.UserHomeDir()
	return &App{
		dbPath: filepath.Join(home, ".monoes", "monoes.db"),
		logs:   make([]LogEntry, 0, 200),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	db, err := sql.Open("sqlite", a.dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		runtime.LogErrorf(ctx, "DB open error: %v", err)
		return
	}
	a.db = db

	// Initialize workflow hybrid store (file + SQLite) so workflows created
	// by both the GUI and the CLI are visible.
	wfDir := filepath.Join(os.Getenv("HOME"), ".monoes", "workflows")
	fileStore, wfErr := workflow.NewWorkflowFileStore(wfDir)
	if wfErr != nil {
		fmt.Printf("workflow file store init error: %v\n", wfErr)
	} else {
		sqlStore := workflow.NewSQLiteWorkflowStore(db)
		a.wfStore = workflow.NewHybridWorkflowStore(fileStore, sqlStore)
	}

	// Initialize connections manager.
	mgr, err := connections.NewManager(a.db)
	if err != nil {
		fmt.Printf("connections manager init error: %v\n", err)
	} else {
		a.connMgr = mgr
	}

	// Initialize config manager for selector resolution in browser nodes.
	cfgLogger := zerolog.New(io.Discard)
	home2, _ := os.UserHomeDir()
	apiClient := config.NewAPIClient(cfgLogger)
	rawCfgMgr := config.NewConfigManager(filepath.Join(home2, ".monoes", "configs"), nil, apiClient, cfgLogger)
	a.cfgMgr = &config.ConfigManagerAdapter{Mgr: rawCfgMgr}

	// Initialize AI store.
	aiStore, aiErr := ai.NewAIStore(db)
	if aiErr != nil {
		fmt.Printf("ai store init error: %v\n", aiErr)
	} else {
		a.aiStore = aiStore
		cs := aichat.NewChatService(aiStore, db)
		// Feed the node type registry into canvas tools so AI knows what nodes are available.
		ntMap := a.GetWorkflowNodeTypes()
		var allTypes []aichat.NodeTypeInfo
		for _, v := range ntMap {
			// v is interface{} wrapping a typed slice; marshal+unmarshal to extract
			b, err := json.Marshal(v)
			if err != nil {
				continue
			}
			var items []aichat.NodeTypeInfo
			if err := json.Unmarshal(b, &items); err != nil {
				continue
			}
			allTypes = append(allTypes, items...)
		}
		cs.SetCanvasNodeTypes(allTypes)
		a.chatService = cs
	}

	// Ensure schema is up-to-date with any columns added by CLI migrations.
	safeMigrations := []string{
		`ALTER TABLE people ADD COLUMN IF NOT EXISTS profile_url TEXT`,
		`CREATE TABLE IF NOT EXISTS tags (
			id    TEXT PRIMARY KEY,
			name  TEXT NOT NULL UNIQUE COLLATE NOCASE,
			color TEXT NOT NULL DEFAULT '#00b4d8'
		)`,
		`CREATE TABLE IF NOT EXISTS people_tags (
			person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
			tag_id    TEXT NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
			PRIMARY KEY (person_id, tag_id)
		)`,
		`ALTER TABLE actions ADD COLUMN IF NOT EXISTS params TEXT NOT NULL DEFAULT '{}'`,
		`CREATE TABLE IF NOT EXISTS workflows (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			is_active   INTEGER NOT NULL DEFAULT 0,
			version     INTEGER NOT NULL DEFAULT 1,
			created_at  TEXT NOT NULL DEFAULT '',
			updated_at  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_nodes (
			id          TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			node_type   TEXT NOT NULL DEFAULT '',
			name        TEXT NOT NULL DEFAULT '',
			config      TEXT NOT NULL DEFAULT '{}',
			position_x  REAL NOT NULL DEFAULT 0,
			position_y  REAL NOT NULL DEFAULT 0,
			disabled    INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL DEFAULT '',
			updated_at  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_connections (
			id             TEXT PRIMARY KEY,
			workflow_id    TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			source_node_id TEXT NOT NULL DEFAULT '',
			source_handle  TEXT NOT NULL DEFAULT '',
			target_node_id TEXT NOT NULL DEFAULT '',
			target_handle  TEXT NOT NULL DEFAULT '',
			position       INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id            TEXT PRIMARY KEY,
			person_id     TEXT REFERENCES people(id),
			platform      TEXT NOT NULL,
			shortcode     TEXT NOT NULL,
			url           TEXT NOT NULL,
			thumbnail_url TEXT,
			like_count    INTEGER,
			comment_count INTEGER,
			caption       TEXT,
			posted_at     TEXT,
			scraped_at    TEXT NOT NULL,
			UNIQUE(platform, shortcode)
		)`,
		`CREATE TABLE IF NOT EXISTS post_comments (
			id          TEXT PRIMARY KEY,
			post_id     TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			author      TEXT NOT NULL,
			text        TEXT,
			timestamp   TEXT,
			likes_count INTEGER DEFAULT 0,
			reply_count INTEGER DEFAULT 0,
			scraped_at  TEXT NOT NULL,
			UNIQUE(post_id, author, timestamp)
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_executions (
			id              TEXT PRIMARY KEY,
			workflow_id     TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			status          TEXT NOT NULL DEFAULT 'QUEUED',
			trigger_type    TEXT NOT NULL DEFAULT 'manual',
			trigger_data    TEXT NOT NULL DEFAULT '{}',
			started_at      TIMESTAMP,
			finished_at     TIMESTAMP,
			error_message   TEXT,
			created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_execution_nodes (
			id              TEXT PRIMARY KEY,
			execution_id    TEXT NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
			node_id         TEXT NOT NULL,
			node_name       TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'PENDING',
			input_items     TEXT NOT NULL DEFAULT '[]',
			output_items    TEXT NOT NULL DEFAULT '[]',
			error_message   TEXT,
			started_at      TIMESTAMP,
			finished_at     TIMESTAMP,
			retry_count     INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS credentials (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			type        TEXT NOT NULL,
			data        TEXT NOT NULL DEFAULT '{}',
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, q := range safeMigrations {
		_, _ = db.Exec(q)
	}

	a.emitLog("SYSTEM", "INFO", "Mono Agent UI connected to "+a.dbPath)
}

func (a *App) shutdown(_ context.Context) {
	if a.db != nil {
		_ = a.db.Close()
	}
}

// newUUID generates a random UUID v4 without external dependencies.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (a *App) emitLog(source, level, message string) {
	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Source:  source,
		Level:   level,
		Message: message,
	}
	a.logs = append(a.logs, entry)
	if len(a.logs) > 500 {
		a.logs = a.logs[len(a.logs)-500:]
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log:entry", entry)
	}
}

// OpenURL opens a URL in the system default browser.
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// ─────────────────────────────────────────────────────────────────────────────
// Dashboard
// ─────────────────────────────────────────────────────────────────────────────

type DashboardStats struct {
	ActiveSessions int                    `json:"active_sessions"`
	TotalActions   int                    `json:"total_actions"`
	ActionsByState map[string]int         `json:"actions_by_state"`
	TotalPeople    int                    `json:"total_people"`
	TotalLists     int                    `json:"total_lists"`
	Sessions       []SessionSummary       `json:"sessions"`
	RecentActions  []ActionInfo           `json:"recent_actions"`
	DBPath         string                 `json:"db_path"`
}

type SessionSummary struct {
	Platform string `json:"platform"`
	Username string `json:"username"`
	Expiry   string `json:"expiry"`
	Active   bool   `json:"active"`
}

func (a *App) GetDashboardStats() DashboardStats {
	stats := DashboardStats{
		ActionsByState: make(map[string]int),
		DBPath:         a.dbPath,
	}
	if a.db == nil {
		return stats
	}

	_ = a.db.QueryRow("SELECT COUNT(*) FROM crawler_sessions WHERE expiry > datetime('now')").Scan(&stats.ActiveSessions)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM people").Scan(&stats.TotalPeople)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM social_lists").Scan(&stats.TotalLists)

	rows, _ := a.db.Query("SELECT state, COUNT(*) FROM actions GROUP BY state")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var state string
			var count int
			if rows.Scan(&state, &count) == nil {
				stats.ActionsByState[state] = count
				stats.TotalActions += count
			}
		}
	}

	sessionRows, _ := a.db.Query(`SELECT platform, username, expiry, (expiry > datetime('now')) as active
	                               FROM crawler_sessions ORDER BY platform`)
	if sessionRows != nil {
		defer sessionRows.Close()
		for sessionRows.Next() {
			var s SessionSummary
			var activeInt int
			if sessionRows.Scan(&s.Platform, &s.Username, &s.Expiry, &activeInt) == nil {
				s.Active = activeInt == 1
				stats.Sessions = append(stats.Sessions, s)
			}
		}
	}

	stats.RecentActions = a.GetActions("", "", 6)
	return stats
}

// ─────────────────────────────────────────────────────────────────────────────
// Actions
// ─────────────────────────────────────────────────────────────────────────────

type ActionInfo struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Type         string                 `json:"type"`
	State        string                 `json:"state"`
	Platform     string                 `json:"platform"`
	Keywords     string                 `json:"keywords"`
	ContentMsg   string                 `json:"content_message"`
	ReachedIndex int                    `json:"reached_index"`
	ExecCount    int                    `json:"exec_count"`
	TargetCount  int                    `json:"target_count"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
	Params       map[string]interface{} `json:"params,omitempty"`
}

func (a *App) GetActions(platform, state string, limit int) []ActionInfo {
	if a.db == nil {
		return nil
	}
	query := `SELECT id, title, type, state, target_platform,
	                 COALESCE(keywords,''), COALESCE(content_message,''),
	                 reached_index, action_execution_count,
	                 COALESCE(created_at_ts,''), COALESCE(updated_at_ts,'')
	          FROM actions WHERE 1=1`
	var args []interface{}

	if platform != "" && platform != "ALL" {
		query += " AND target_platform = ?"
		args = append(args, strings.ToUpper(platform))
	}
	if state != "" && state != "ALL" {
		query += " AND state = ?"
		args = append(args, strings.ToUpper(state))
	}
	query += " ORDER BY created_at_ts DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var actions []ActionInfo
	for rows.Next() {
		var act ActionInfo
		if rows.Scan(&act.ID, &act.Title, &act.Type, &act.State, &act.Platform,
			&act.Keywords, &act.ContentMsg, &act.ReachedIndex, &act.ExecCount,
			&act.CreatedAt, &act.UpdatedAt) == nil {
			_ = a.db.QueryRow("SELECT COUNT(*) FROM action_targets WHERE action_id = ?", act.ID).Scan(&act.TargetCount)
			actions = append(actions, act)
		}
	}
	return actions
}

func (a *App) GetAction(id string) *ActionInfo {
	if a.db == nil {
		return nil
	}
	row := a.db.QueryRow(`SELECT id, title, type, state, target_platform,
	                             COALESCE(keywords,''), COALESCE(content_message,''),
	                             reached_index, action_execution_count,
	                             COALESCE(created_at_ts,''), COALESCE(updated_at_ts,''),
	                             COALESCE(params,'{}')
	                      FROM actions WHERE id = ?`, id)
	var act ActionInfo
	var paramsJSON string
	if row.Scan(&act.ID, &act.Title, &act.Type, &act.State, &act.Platform,
		&act.Keywords, &act.ContentMsg, &act.ReachedIndex, &act.ExecCount,
		&act.CreatedAt, &act.UpdatedAt, &paramsJSON) != nil {
		return nil
	}
	if paramsJSON != "" && paramsJSON != "{}" {
		var p map[string]interface{}
		if json.Unmarshal([]byte(paramsJSON), &p) == nil {
			act.Params = p
		}
	}
	_ = a.db.QueryRow("SELECT COUNT(*) FROM action_targets WHERE action_id = ?", act.ID).Scan(&act.TargetCount)
	return &act
}

type CreateActionRequest struct {
	Title          string                 `json:"title"`
	Type           string                 `json:"type"`
	Platform       string                 `json:"platform"`
	Keywords       string                 `json:"keywords"`
	ContentMessage string                 `json:"content_message"`
	Params         map[string]interface{} `json:"params,omitempty"`
}

func (a *App) CreateAction(req CreateActionRequest) (*ActionInfo, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	id := newUUID()
	now := time.Now()
	paramsJSON := "{}"
	if len(req.Params) > 0 {
		if b, err := json.Marshal(req.Params); err == nil {
			paramsJSON = string(b)
		}
	}
	_, err := a.db.Exec(`INSERT INTO actions
	                      (id, created_at, title, type, state, target_platform, keywords, content_message, params, created_at_ts, updated_at_ts)
	                      VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?, ?, ?, ?)`,
		id, now.Unix(), req.Title, strings.ToUpper(req.Type), strings.ToUpper(req.Platform),
		req.Keywords, req.ContentMessage, paramsJSON, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	a.emitLog("ACTIONS", "INFO", fmt.Sprintf("Created action: %s [%s/%s]", req.Title, req.Platform, req.Type))
	return &ActionInfo{
		ID:       id,
		Title:    req.Title,
		Type:     strings.ToUpper(req.Type),
		State:    "PENDING",
		Platform: strings.ToUpper(req.Platform),
		Keywords: req.Keywords,
		Params:   req.Params,
	}, nil
}

func (a *App) UpdateActionState(id, state string) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	_, err := a.db.Exec("UPDATE actions SET state = ?, updated_at_ts = ? WHERE id = ?",
		strings.ToUpper(state), time.Now().Format(time.RFC3339), id)
	return err
}

func (a *App) UpdateActionParams(id string, params map[string]interface{}) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	paramsJSON := "{}"
	if len(params) > 0 {
		if b, err := json.Marshal(params); err == nil {
			paramsJSON = string(b)
		}
	}
	_, err := a.db.Exec("UPDATE actions SET params = ?, updated_at_ts = ? WHERE id = ?",
		paramsJSON, time.Now().Format(time.RFC3339), id)
	return err
}

func (a *App) DeleteAction(id string) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	_, err := a.db.Exec("DELETE FROM actions WHERE id = ?", id)
	if err == nil {
		a.emitLog("ACTIONS", "WARN", "Deleted action: "+id)
	}
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Targets
// ─────────────────────────────────────────────────────────────────────────────

type TargetInfo struct {
	ID        string `json:"id"`
	ActionID  string `json:"action_id"`
	Platform  string `json:"platform"`
	Link      string `json:"link"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (a *App) GetActionTargets(actionID string) []TargetInfo {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query(`SELECT id, action_id, platform, COALESCE(link,''), status, COALESCE(created_at,'')
	                          FROM action_targets WHERE action_id = ? ORDER BY created_at DESC`, actionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var targets []TargetInfo
	for rows.Next() {
		var t TargetInfo
		if rows.Scan(&t.ID, &t.ActionID, &t.Platform, &t.Link, &t.Status, &t.CreatedAt) == nil {
			targets = append(targets, t)
		}
	}
	return targets
}

func (a *App) AddActionTarget(actionID, link, platform string) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	id := newUUID()
	_, err := a.db.Exec(`INSERT INTO action_targets (id, action_id, platform, link, status) VALUES (?, ?, ?, ?, 'PENDING')`,
		id, actionID, strings.ToUpper(platform), link)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// People
// ─────────────────────────────────────────────────────────────────────────────

type PersonInfo struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Platform       string `json:"platform"`
	FullName       string `json:"full_name"`
	ImageURL       string `json:"image_url"`
	ProfileURL     string `json:"profile_url"`
	FollowerCount  string `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
	IsVerified     bool   `json:"is_verified"`
	JobTitle       string `json:"job_title"`
	Category       string `json:"category"`
	CreatedAt      string `json:"created_at"`
}

type PersonDetailInfo struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Platform       string `json:"platform"`
	FullName       string `json:"full_name"`
	ImageURL       string `json:"image_url"`
	ProfileURL     string `json:"profile_url"`
	FollowerCount  string `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
	ContentCount   int    `json:"content_count"`
	IsVerified     bool   `json:"is_verified"`
	JobTitle       string `json:"job_title"`
	Category       string `json:"category"`
	Introduction   string `json:"introduction"`
	Website        string `json:"website"`
	ContactDetails string `json:"contact_details"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type PersonInteraction struct {
	ActionID         string `json:"action_id"`
	ActionTitle      string `json:"action_title"`
	ActionType       string `json:"action_type"`
	Platform         string `json:"platform"`
	Link             string `json:"link"`
	Status           string `json:"status"`
	CommentText      string `json:"comment_text"`
	SourceType       string `json:"source_type"`
	LastInteractedAt string `json:"last_interacted_at"`
	CreatedAt        string `json:"created_at"`
}

// PostSummary is returned by GetPersonPosts.
type PostSummary struct {
	ID           string `json:"id"`
	Shortcode    string `json:"shortcode"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url"`
	LikeCount    int    `json:"like_count"`
	CommentCount int    `json:"comment_count"`
	Caption      string `json:"caption"`
	PostedAt     string `json:"posted_at"`
	ScrapedAt    string `json:"scraped_at"`
	WeLiked      bool   `json:"we_liked"`
	WeCommented  bool   `json:"we_commented"`
}

// PostDetail is returned by GetPostDetail.
type PostDetail struct {
	ID           string `json:"id"`
	Shortcode    string `json:"shortcode"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url"`
	LikeCount    int    `json:"like_count"`
	CommentCount int    `json:"comment_count"`
	Caption      string `json:"caption"`
	PostedAt     string `json:"posted_at"`
	ScrapedAt    string `json:"scraped_at"`
}

// PostComment is returned by GetPostComments.
type PostComment struct {
	ID         string `json:"id"`
	Author     string `json:"author"`
	Text       string `json:"text"`
	Timestamp  string `json:"timestamp"`
	LikesCount int    `json:"likes_count"`
	ReplyCount int    `json:"reply_count"`
}

func (a *App) GetPeople(platform, search string, limit, offset int) []PersonInfo {
	if a.db == nil {
		return nil
	}
	query := `SELECT id, platform_username, platform, COALESCE(full_name,''), COALESCE(image_url,''),
	                 COALESCE(profile_url,''), COALESCE(follower_count,''), COALESCE(following_count,0), COALESCE(is_verified,0),
	                 COALESCE(job_title,''), COALESCE(category,''), COALESCE(created_at,'')
	          FROM people WHERE 1=1`
	var args []interface{}
	if platform != "" && platform != "ALL" {
		query += " AND UPPER(platform) = ?"
		args = append(args, strings.ToUpper(platform))
	}
	if search != "" {
		query += " AND (platform_username LIKE ? OR full_name LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s)
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var people []PersonInfo
	for rows.Next() {
		var p PersonInfo
		var isVerified int
		if rows.Scan(&p.ID, &p.Username, &p.Platform, &p.FullName, &p.ImageURL,
			&p.ProfileURL, &p.FollowerCount, &p.FollowingCount, &isVerified, &p.JobTitle, &p.Category, &p.CreatedAt) == nil {
			p.IsVerified = isVerified == 1
			people = append(people, p)
		}
	}
	return people
}

func (a *App) GetPeopleCount(platform, search string) int {
	if a.db == nil {
		return 0
	}
	query := "SELECT COUNT(*) FROM people WHERE 1=1"
	var args []interface{}
	if platform != "" && platform != "ALL" {
		query += " AND UPPER(platform) = ?"
		args = append(args, strings.ToUpper(platform))
	}
	if search != "" {
		query += " AND (platform_username LIKE ? OR full_name LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s)
	}
	var count int
	_ = a.db.QueryRow(query, args...).Scan(&count)
	return count
}

func (a *App) GetPersonDetail(id string) *PersonDetailInfo {
	if a.db == nil {
		return nil
	}
	row := a.db.QueryRow(`
		SELECT id, platform_username, platform,
		       COALESCE(full_name,''), COALESCE(image_url,''), COALESCE(profile_url,''),
		       COALESCE(follower_count,''), COALESCE(following_count,0), COALESCE(content_count,0), COALESCE(is_verified,0),
		       COALESCE(job_title,''), COALESCE(category,''),
		       COALESCE(introduction,''), COALESCE(website,''), COALESCE(contact_details,''),
		       COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM people WHERE id = ?`, id)
	var p PersonDetailInfo
	var isVerified int
	if err := row.Scan(&p.ID, &p.Username, &p.Platform,
		&p.FullName, &p.ImageURL, &p.ProfileURL,
		&p.FollowerCount, &p.FollowingCount, &p.ContentCount, &isVerified,
		&p.JobTitle, &p.Category,
		&p.Introduction, &p.Website, &p.ContactDetails,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil
	}
	p.IsVerified = isVerified == 1
	return &p
}

func (a *App) GetPersonInteractions(id string) []PersonInteraction {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query(`
		SELECT at.action_id, COALESCE(a.title,''), COALESCE(a.type,''),
		       at.platform, COALESCE(at.link,''), at.status,
		       COALESCE(at.comment_text,''), COALESCE(at.source_type,''),
		       COALESCE(at.last_interacted_at,''), COALESCE(at.created_at,'')
		FROM action_targets at
		LEFT JOIN actions a ON at.action_id = a.id
		WHERE at.person_id = ?
		ORDER BY COALESCE(at.last_interacted_at, at.created_at) DESC
		LIMIT 200`, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var interactions []PersonInteraction
	for rows.Next() {
		var i PersonInteraction
		if rows.Scan(&i.ActionID, &i.ActionTitle, &i.ActionType,
			&i.Platform, &i.Link, &i.Status,
			&i.CommentText, &i.SourceType,
			&i.LastInteractedAt, &i.CreatedAt) == nil {
			interactions = append(interactions, i)
		}
	}
	return interactions
}

// GetPersonPosts returns all scraped posts for a person, with we_liked/we_commented flags.
func (a *App) GetPersonPosts(personID string) []PostSummary {
	if a.db == nil {
		return []PostSummary{}
	}
	rows, err := a.db.Query(`
		SELECT
			p.id,
			p.shortcode,
			p.url,
			COALESCE(p.thumbnail_url, ''),
			COALESCE(p.like_count, 0),
			COALESCE(p.comment_count, 0),
			COALESCE(p.caption, ''),
			COALESCE(p.posted_at, ''),
			p.scraped_at,
			EXISTS(
				SELECT 1 FROM action_targets at2
				JOIN actions a2 ON at2.action_id = a2.id
				WHERE rtrim(at2.link, '/') = rtrim(p.url, '/')
				  AND a2.type = 'like_posts'
				  AND at2.status = 'COMPLETED'
			) AS we_liked,
			EXISTS(
				SELECT 1 FROM action_targets at3
				JOIN actions a3 ON at3.action_id = a3.id
				WHERE rtrim(at3.link, '/') = rtrim(p.url, '/')
				  AND a3.type = 'comment_on_posts'
				  AND at3.status = 'COMPLETED'
			) AS we_commented
		FROM posts p
		WHERE p.person_id = ?
		ORDER BY p.scraped_at DESC`,
		personID,
	)
	if err != nil {
		return []PostSummary{}
	}
	defer rows.Close()

	var posts []PostSummary
	for rows.Next() {
		var p PostSummary
		var weLiked, weCommented int
		if err := rows.Scan(
			&p.ID, &p.Shortcode, &p.URL, &p.ThumbnailURL,
			&p.LikeCount, &p.CommentCount, &p.Caption,
			&p.PostedAt, &p.ScrapedAt,
			&weLiked, &weCommented,
		); err != nil {
			continue
		}
		p.WeLiked = weLiked != 0
		p.WeCommented = weCommented != 0
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return []PostSummary{}
	}
	if posts == nil {
		return []PostSummary{}
	}
	return posts
}

// GetPostDetail returns full metadata for a single post by ID.
func (a *App) GetPostDetail(postID string) *PostDetail {
	if a.db == nil {
		return nil
	}
	var p PostDetail
	err := a.db.QueryRow(`
		SELECT id, shortcode, url,
		       COALESCE(thumbnail_url, ''),
		       COALESCE(like_count, 0),
		       COALESCE(comment_count, 0),
		       COALESCE(caption, ''),
		       COALESCE(posted_at, ''),
		       scraped_at
		FROM posts WHERE id = ?`,
		postID,
	).Scan(
		&p.ID, &p.Shortcode, &p.URL, &p.ThumbnailURL,
		&p.LikeCount, &p.CommentCount, &p.Caption,
		&p.PostedAt, &p.ScrapedAt,
	)
	if err != nil {
		return nil
	}
	return &p
}

// GetPostComments returns all scraped comments for a post, ordered by timestamp.
func (a *App) GetPostComments(postID string) []PostComment {
	if a.db == nil {
		return []PostComment{}
	}
	rows, err := a.db.Query(`
		SELECT id, COALESCE(author, ''), COALESCE(text, ''),
		       COALESCE(timestamp, ''),
		       COALESCE(likes_count, 0),
		       COALESCE(reply_count, 0)
		FROM post_comments
		WHERE post_id = ?
		ORDER BY timestamp ASC`,
		postID,
	)
	if err != nil {
		return []PostComment{}
	}
	defer rows.Close()

	var comments []PostComment
	for rows.Next() {
		var c PostComment
		if err := rows.Scan(
			&c.ID, &c.Author, &c.Text,
			&c.Timestamp, &c.LikesCount, &c.ReplyCount,
		); err != nil {
			continue
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return []PostComment{}
	}
	if comments == nil {
		return []PostComment{}
	}
	return comments
}

// ─────────────────────────────────────────────────────────────────────────────
// Tags
// ─────────────────────────────────────────────────────────────────────────────

type TagInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// GetAllTags returns every tag in the system, ordered by name.
func (a *App) GetAllTags() []TagInfo {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query(`SELECT id, name, color FROM tags ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []TagInfo
	for rows.Next() {
		var t TagInfo
		if rows.Scan(&t.ID, &t.Name, &t.Color) == nil {
			tags = append(tags, t)
		}
	}
	return tags
}

// GetPersonTags returns all tags attached to the given person.
func (a *App) GetPersonTags(personId string) []TagInfo {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query(`
		SELECT t.id, t.name, t.color
		FROM tags t
		JOIN people_tags pt ON pt.tag_id = t.id
		WHERE pt.person_id = ?
		ORDER BY t.name COLLATE NOCASE`, personId)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []TagInfo
	for rows.Next() {
		var t TagInfo
		if rows.Scan(&t.ID, &t.Name, &t.Color) == nil {
			tags = append(tags, t)
		}
	}
	return tags
}

// AddPersonTag creates a tag (if new) and links it to the person.
// Returns the tag that was added, or nil on error / if the person already has 10 tags.
func (a *App) AddPersonTag(personId, tagName, color string) *TagInfo {
	if a.db == nil {
		return nil
	}
	tagName = strings.TrimSpace(tagName)
	if tagName == "" {
		return nil
	}

	// Enforce max-10 limit.
	var count int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM people_tags WHERE person_id = ?`, personId).Scan(&count)
	if count >= 10 {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return nil
	}
	defer tx.Rollback()

	// Find or create the tag.
	var tagId, tagColor string
	err = tx.QueryRow(`SELECT id, color FROM tags WHERE LOWER(name) = LOWER(?)`, tagName).Scan(&tagId, &tagColor)
	if err != nil {
		// Create new tag.
		tagId = newUUID()
		if color == "" {
			color = "#00b4d8"
		}
		if _, err = tx.Exec(`INSERT INTO tags(id, name, color) VALUES(?,?,?)`, tagId, tagName, color); err != nil {
			return nil
		}
		tagColor = color
	}

	// Link person ↔ tag (ignore if already linked).
	if _, err = tx.Exec(`INSERT OR IGNORE INTO people_tags(person_id, tag_id) VALUES(?,?)`, personId, tagId); err != nil {
		return nil
	}

	if err = tx.Commit(); err != nil {
		return nil
	}
	return &TagInfo{ID: tagId, Name: tagName, Color: tagColor}
}

// RemovePersonTag unlinks a tag from a person (does not delete the tag globally).
func (a *App) RemovePersonTag(personId, tagId string) {
	if a.db == nil {
		return
	}
	_, _ = a.db.Exec(`DELETE FROM people_tags WHERE person_id = ? AND tag_id = ?`, personId, tagId)
}

// GetPeopleTagsMap returns a map of personId → []TagInfo for a slice of person IDs.
// Used to bulk-load tags for the People list without N queries.
func (a *App) GetPeopleTagsMap(personIds []string) map[string][]TagInfo {
	if a.db == nil || len(personIds) == 0 {
		return nil
	}

	// Build IN clause.
	placeholders := make([]string, len(personIds))
	args := make([]interface{}, len(personIds))
	for i, id := range personIds {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT pt.person_id, t.id, t.name, t.color
		FROM people_tags pt
		JOIN tags t ON t.id = pt.tag_id
		WHERE pt.person_id IN (%s)
		ORDER BY t.name COLLATE NOCASE`, strings.Join(placeholders, ","))

	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make(map[string][]TagInfo)
	for rows.Next() {
		var pid string
		var t TagInfo
		if rows.Scan(&pid, &t.ID, &t.Name, &t.Color) == nil {
			result[pid] = append(result[pid], t)
		}
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Sessions
// ─────────────────────────────────────────────────────────────────────────────

type SessionInfo struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Platform string `json:"platform"`
	Expiry   string `json:"expiry"`
	AddedAt  string `json:"added_at"`
	Active   bool   `json:"active"`
}

func (a *App) GetSessions() []SessionInfo {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query(`SELECT id, username, platform, expiry, when_added,
	                                (expiry > datetime('now')) as active
	                          FROM crawler_sessions ORDER BY platform, username`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var sessions []SessionInfo
	for rows.Next() {
		var s SessionInfo
		var activeInt int
		if rows.Scan(&s.ID, &s.Username, &s.Platform, &s.Expiry, &s.AddedAt, &activeInt) == nil {
			s.Active = activeInt == 1
			sessions = append(sessions, s)
		}
	}
	return sessions
}

func (a *App) DeleteSession(id int) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	_, err := a.db.Exec("DELETE FROM crawler_sessions WHERE id = ?", id)
	if err == nil {
		a.emitLog("SESSIONS", "WARN", fmt.Sprintf("Deleted session ID %d", id))
	}
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Social Lists
// ─────────────────────────────────────────────────────────────────────────────

type SocialListInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ListType  string `json:"list_type"`
	ItemCount int    `json:"item_count"`
	CreatedAt string `json:"created_at"`
}

func (a *App) GetSocialLists() []SocialListInfo {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query(`SELECT id, name, COALESCE(list_type,''), item_count, COALESCE(created_at,'')
	                          FROM social_lists ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var lists []SocialListInfo
	for rows.Next() {
		var l SocialListInfo
		if rows.Scan(&l.ID, &l.Name, &l.ListType, &l.ItemCount, &l.CreatedAt) == nil {
			lists = append(lists, l)
		}
	}
	return lists
}

// ─────────────────────────────────────────────────────────────────────────────
// Templates
// ─────────────────────────────────────────────────────────────────────────────

type TemplateInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (a *App) GetTemplates() []TemplateInfo {
	if a.db == nil {
		return nil
	}
	rows, err := a.db.Query("SELECT id, name, COALESCE(subject,''), body FROM templates ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var templates []TemplateInfo
	for rows.Next() {
		var t TemplateInfo
		if rows.Scan(&t.ID, &t.Name, &t.Subject, &t.Body) == nil {
			templates = append(templates, t)
		}
	}
	return templates
}

// ─────────────────────────────────────────────────────────────────────────────
// Action Execution
// ─────────────────────────────────────────────────────────────────────────────

// findMonoesBinary locates the monoes CLI binary by checking PATH and common
// install locations, since macOS GUI apps don't inherit the shell PATH.
func findMonoesBinary() (string, error) {
	// 1. Check PATH (works in terminal / dev mode)
	if p, err := exec.LookPath("monoes"); err == nil {
		return p, nil
	}

	// 2. Same directory as the running binary (bundled alongside the app)
	if execDir, err := filepath.Abs(filepath.Dir(os.Args[0])); err == nil {
		if p := filepath.Join(execDir, "monoes"); fileExists(p) {
			return p, nil
		}
	}

	// 3. Common user-level install locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "go", "bin", "monoes"),        // go install default
		filepath.Join(home, ".local", "bin", "monoes"),
		"/usr/local/bin/monoes",
		"/opt/homebrew/bin/monoes",
		"/usr/bin/monoes",
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p, nil
		}
	}

	return "", fmt.Errorf("monoes binary not found — tried PATH, ~/go/bin, /usr/local/bin, /opt/homebrew/bin. Run `go install` or place the binary alongside this app")
}

func fileExists(p string) bool {
	info, err := os.Stat(p) // Stat follows symlinks; returns error for broken symlinks
	return err == nil && !info.IsDir()
}

// nodeTypeToPlatform derives the connection platform ID from a node type string.
// e.g. "service.google_sheets" → "google_sheets", "db.postgres" → "postgresql"
func nodeTypeToPlatform(nodeType string) string {
	overrides := map[string]string{
		"db.postgres": "postgresql",
		"db.mysql":    "mysql",
		"db.mongodb":  "mongodb",
		"db.redis":    "redis",
		"comm.email_send": "smtp",
		"comm.email_read": "imap",
	}
	if p, ok := overrides[nodeType]; ok {
		return p
	}
	parts := strings.SplitN(nodeType, ".", 2)
	if len(parts) == 2 {
		return parts[1] // "service.google_sheets" → "google_sheets"
	}
	return nodeType
}

func (a *App) ExecuteAction(id string) error {
	monoesBin, err := findMonoesBinary()
	if err != nil {
		return err
	}

	_ = a.db.QueryRow("UPDATE actions SET state = 'RUNNING', updated_at_ts = ? WHERE id = ?",
		time.Now().Format(time.RFC3339), id)

	cmd := exec.CommandContext(a.ctx, monoesBin, "run", id, "--verbose")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start action: %w", err)
	}
	a.emitLog("RUNNER", "INFO", fmt.Sprintf("Started action %s", id))

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			a.emitLog("STDOUT", "INFO", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			a.emitLog("STDERR", "WARN", scanner.Text())
		}
	}()
	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			a.emitLog("RUNNER", "ERROR", fmt.Sprintf("Action %s failed: %v", id, waitErr))
			runtime.EventsEmit(a.ctx, "action:complete", map[string]interface{}{"action_id": id, "success": false})
		} else {
			a.emitLog("RUNNER", "INFO", fmt.Sprintf("Action %s completed successfully", id))
			runtime.EventsEmit(a.ctx, "action:complete", map[string]interface{}{"action_id": id, "success": true})
		}
	}()
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Logs
// ─────────────────────────────────────────────────────────────────────────────

type LogEntry struct {
	Time    string `json:"time"`
	Source  string `json:"source"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (a *App) GetLogs() []LogEntry {
	return a.logs
}

func (a *App) ClearLogs() {
	a.logs = make([]LogEntry, 0, 200)
}

// ─────────────────────────────────────────────────────────────────────────────
// Metadata
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) GetAvailableActionTypes() map[string][]string {
	return map[string][]string{
		"INSTAGRAM": {
			"find_by_keyword", "export_followers", "scrape_profile_info", "engage_with_posts",
			"send_dms", "auto_reply_dms", "publish_post",
			"like_posts", "comment_on_posts", "like_comments_on_posts", "extract_post_data",
			"follow_users", "unfollow_users", "watch_stories", "engage_user_posts",
		},
		"LINKEDIN": {
			"find_by_keyword", "export_followers", "scrape_profile_info", "engage_with_posts",
			"send_dms", "auto_reply_dms", "publish_post",
		},
		"X": {
			"find_by_keyword", "export_followers", "scrape_profile_info", "engage_with_posts",
			"send_dms", "auto_reply_dms", "publish_post",
		},
		"TIKTOK": {
			"find_by_keyword", "export_followers", "scrape_profile_info", "engage_with_posts",
			"send_dms", "auto_reply_dms", "publish_post",
		},
	}
}

func (a *App) GetDBPath() string {
	return a.dbPath
}

func (a *App) IsDBConnected() bool {
	if a.db == nil {
		return false
	}
	return a.db.Ping() == nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow types
// ─────────────────────────────────────────────────────────────────────────────

type WorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	Version     int    `json:"version"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type WorkflowNodeData struct {
	ID        string                 `json:"id"`
	NodeType  string                 `json:"node_type"`
	Name      string                 `json:"name"`
	Config    map[string]interface{} `json:"config"`
	PositionX float64                `json:"position_x"`
	PositionY float64                `json:"position_y"`
	Disabled  bool                   `json:"disabled"`
	Schema    *workflow.NodeSchema   `json:"schema,omitempty"`
}

type WorkflowConnectionData struct {
	ID           string `json:"id"`
	SourceNodeID string `json:"source_node_id"`
	SourceHandle string `json:"source_handle"`
	TargetNodeID string `json:"target_node_id"`
	TargetHandle string `json:"target_handle"`
	Position     int    `json:"position"`
}

type WorkflowDetail struct {
	WorkflowSummary
	Nodes       []WorkflowNodeData       `json:"nodes"`
	Connections []WorkflowConnectionData `json:"connections"`
}

type SaveWorkflowRequest struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	IsActive    bool                     `json:"is_active"`
	Nodes       []WorkflowNodeData       `json:"nodes"`
	Connections []WorkflowConnectionData `json:"connections"`
}

type WorkflowExecutionSummary struct {
	ID           string `json:"id"`
	WorkflowID   string `json:"workflow_id"`
	WorkflowName string `json:"workflow_name"`
	Status       string `json:"status"`
	TriggerType  string `json:"trigger_type"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Error        string `json:"error"`
	CreatedAt    string `json:"created_at"`
}

type CredentialSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ServiceType string `json:"service_type"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type SaveCredentialRequest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	ServiceType string                 `json:"service_type"`
	Data        map[string]interface{} `json:"data"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow CRUD
// ─────────────────────────────────────────────────────────────────────────────

// workflowToDetail converts a *workflow.Workflow into a *WorkflowDetail for the frontend.
func workflowToDetail(wf *workflow.Workflow) *WorkflowDetail {
	detail := &WorkflowDetail{
		WorkflowSummary: WorkflowSummary{
			ID:          wf.ID,
			Name:        wf.Name,
			Description: wf.Description,
			IsActive:    wf.IsActive,
			Version:     wf.Version,
			CreatedAt:   wf.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   wf.UpdatedAt.Format(time.RFC3339),
		},
		Nodes:       []WorkflowNodeData{},
		Connections: []WorkflowConnectionData{},
	}
	for _, n := range wf.Nodes {
		detail.Nodes = append(detail.Nodes, WorkflowNodeData{
			ID:        n.ID,
			NodeType:  n.Type,
			Name:      n.Name,
			Config:    n.Config,
			PositionX: n.PositionX,
			PositionY: n.PositionY,
			Disabled:  n.Disabled,
			Schema:    n.Schema,
		})
	}
	for _, c := range wf.Connections {
		detail.Connections = append(detail.Connections, WorkflowConnectionData{
			ID:           c.ID,
			SourceNodeID: c.SourceNodeID,
			SourceHandle: c.SourceHandle,
			TargetNodeID: c.TargetNodeID,
			TargetHandle: c.TargetHandle,
			Position:     c.Position,
		})
	}
	return detail
}

func (a *App) ListWorkflows() ([]WorkflowSummary, error) {
	if a.wfStore == nil {
		return nil, fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wfs, err := a.wfStore.ListWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]WorkflowSummary, 0, len(wfs))
	for i := range wfs {
		summaries = append(summaries, WorkflowSummary{
			ID:          wfs[i].ID,
			Name:        wfs[i].Name,
			Description: wfs[i].Description,
			IsActive:    wfs[i].IsActive,
			Version:     wfs[i].Version,
			CreatedAt:   wfs[i].CreatedAt.Format(time.RFC3339),
			UpdatedAt:   wfs[i].UpdatedAt.Format(time.RFC3339),
		})
	}
	return summaries, nil
}

func (a *App) GetWorkflow(id string) (*WorkflowDetail, error) {
	if a.wfStore == nil {
		return nil, fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wf, err := a.wfStore.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}
	if wf == nil {
		return nil, fmt.Errorf("workflow %s not found", id)
	}
	return workflowToDetail(wf), nil
}

func (a *App) SaveWorkflow(req SaveWorkflowRequest) (*WorkflowSummary, error) {
	if a.wfStore == nil {
		return nil, fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wf := &workflow.Workflow{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    req.IsActive,
	}
	for _, n := range req.Nodes {
		node := workflow.WorkflowNode{
			ID:        n.ID,
			Type:      n.NodeType,
			Name:      n.Name,
			PositionX: n.PositionX,
			PositionY: n.PositionY,
			Disabled:  n.Disabled,
			Config:    n.Config,
			Schema:    n.Schema,
		}
		if node.Schema == nil {
			schema, _ := workflow.LoadDefaultSchema(node.Type)
			node.Schema = schema
		}
		wf.Nodes = append(wf.Nodes, node)
	}
	for _, c := range req.Connections {
		wf.Connections = append(wf.Connections, workflow.WorkflowConnection{
			ID:           c.ID,
			SourceNodeID: c.SourceNodeID,
			SourceHandle: c.SourceHandle,
			TargetNodeID: c.TargetNodeID,
			TargetHandle: c.TargetHandle,
			Position:     c.Position,
		})
	}
	if err := a.wfStore.SaveWorkflow(ctx, wf); err != nil {
		return nil, err
	}
	a.emitLog("WORKFLOW", "INFO", fmt.Sprintf("Saved workflow: %s [%s]", wf.Name, wf.ID))
	return &WorkflowSummary{
		ID:          wf.ID,
		Name:        wf.Name,
		Description: wf.Description,
		IsActive:    wf.IsActive,
		Version:     wf.Version,
		CreatedAt:   wf.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   wf.UpdatedAt.Format(time.RFC3339),
	}, nil
}

func (a *App) DeleteWorkflow(id string) error {
	if a.wfStore == nil {
		return fmt.Errorf("workflow store not available")
	}
	err := a.wfStore.DeleteWorkflow(context.Background(), id)
	if err == nil {
		a.emitLog("WORKFLOW", "WARN", "Deleted workflow: "+id)
	}
	return err
}

func (a *App) SetWorkflowActive(id string, active bool) error {
	if a.wfStore == nil {
		return fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wf, err := a.wfStore.GetWorkflow(ctx, id)
	if err != nil || wf == nil {
		return fmt.Errorf("workflow %s not found", id)
	}
	wf.IsActive = active
	return a.wfStore.SaveWorkflow(ctx, wf)
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow execution (via CLI subprocess)
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) RunWorkflow(id string) error {
	monoesBin, err := findMonoesBinary()
	if err != nil {
		return err
	}

	cmd := exec.Command(monoesBin, "workflow", "run", id)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start workflow %s: %w", id, err)
	}
	a.emitLog("WORKFLOW", "INFO", fmt.Sprintf("Started workflow run: %s", id))

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			a.emitLog("WORKFLOW", "INFO", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			a.emitLog("WORKFLOW", "WARN", scanner.Text())
		}
	}()
	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			a.emitLog("WORKFLOW", "ERROR", fmt.Sprintf("Workflow %s run failed: %v", id, waitErr))
			runtime.EventsEmit(a.ctx, "workflow:complete", map[string]interface{}{"workflow_id": id, "success": false})
		} else {
			a.emitLog("WORKFLOW", "INFO", fmt.Sprintf("Workflow %s run completed successfully", id))
			runtime.EventsEmit(a.ctx, "workflow:complete", map[string]interface{}{"workflow_id": id, "success": true})
		}
	}()
	return nil
}

func (a *App) GetWorkflowExecutions(workflowID string, limit int) ([]WorkflowExecutionSummary, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := a.db.Query(`SELECT id, workflow_id, status, trigger_type,
	                                 COALESCE(started_at, '') as started_at,
	                                 COALESCE(finished_at, '') as finished_at,
	                                 COALESCE(error_message, '') as error,
	                                 created_at
	                          FROM workflow_executions
	                          WHERE workflow_id = ?
	                          ORDER BY created_at DESC
	                          LIMIT ?`, workflowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var execs []WorkflowExecutionSummary
	for rows.Next() {
		var e WorkflowExecutionSummary
		if rows.Scan(&e.ID, &e.WorkflowID, &e.Status, &e.TriggerType,
			&e.StartedAt, &e.FinishedAt, &e.Error, &e.CreatedAt) == nil {
			execs = append(execs, e)
		}
	}
	if execs == nil {
		execs = []WorkflowExecutionSummary{}
	}
	return execs, rows.Err()
}

func (a *App) GetRecentExecutions(limit int) ([]WorkflowExecutionSummary, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := a.db.Query(`SELECT e.id, e.workflow_id, COALESCE(w.name,'') as workflow_name,
	                                 e.status, COALESCE(e.trigger_type,''),
	                                 COALESCE(e.started_at,'') as started_at,
	                                 COALESCE(e.finished_at,'') as finished_at,
	                                 COALESCE(e.error_message,'') as error,
	                                 e.created_at
	                          FROM workflow_executions e
	                          LEFT JOIN workflows w ON e.workflow_id = w.id
	                          ORDER BY e.created_at DESC
	                          LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var execs []WorkflowExecutionSummary
	for rows.Next() {
		var e WorkflowExecutionSummary
		if rows.Scan(&e.ID, &e.WorkflowID, &e.WorkflowName, &e.Status, &e.TriggerType,
			&e.StartedAt, &e.FinishedAt, &e.Error, &e.CreatedAt) == nil {
			execs = append(execs, e)
		}
	}
	if execs == nil {
		execs = []WorkflowExecutionSummary{}
	}
	return execs, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Credentials
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) ListCredentials() ([]CredentialSummary, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	rows, err := a.db.Query(`SELECT id, name, service_type, created_at, updated_at
	                          FROM credentials ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []CredentialSummary
	for rows.Next() {
		var c CredentialSummary
		if rows.Scan(&c.ID, &c.Name, &c.ServiceType, &c.CreatedAt, &c.UpdatedAt) == nil {
			creds = append(creds, c)
		}
	}
	if creds == nil {
		creds = []CredentialSummary{}
	}
	return creds, rows.Err()
}

func (a *App) SaveCredential(req SaveCredentialRequest) (*CredentialSummary, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	dataJSON := "{}"
	if len(req.Data) > 0 {
		if b, err := json.Marshal(req.Data); err == nil {
			dataJSON = string(b)
		}
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	var credID string

	if req.ID == "" {
		credID = uuid.New().String()
		_, err := a.db.Exec(`INSERT INTO credentials (id, name, service_type, encrypted_data, created_at, updated_at)
		                      VALUES (?, ?, ?, ?, ?, ?)`,
			credID, req.Name, req.ServiceType, dataJSON, now, now)
		if err != nil {
			return nil, fmt.Errorf("insert credential: %w", err)
		}
	} else {
		credID = req.ID
		_, err := a.db.Exec(`UPDATE credentials SET name = ?, service_type = ?, encrypted_data = ?, updated_at = ?
		                      WHERE id = ?`,
			req.Name, req.ServiceType, dataJSON, now, credID)
		if err != nil {
			return nil, fmt.Errorf("update credential: %w", err)
		}
	}

	row := a.db.QueryRow(`SELECT id, name, service_type, created_at, updated_at FROM credentials WHERE id = ?`, credID)
	var c CredentialSummary
	if err := row.Scan(&c.ID, &c.Name, &c.ServiceType, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *App) DeleteCredential(id string) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	_, err := a.db.Exec(`DELETE FROM credentials WHERE id = ?`, id)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Node types registry
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) GetWorkflowNodeTypes() map[string]interface{} {
	type nodeDesc struct {
		Type        string               `json:"type"`
		Label       string               `json:"label"`
		Category    string               `json:"category"`
		Description string               `json:"description"`
		Schema      *workflow.NodeSchema `json:"schema,omitempty"`
	}
	mkNode := func(t, label, cat, desc string) nodeDesc {
		schema, _ := workflow.LoadDefaultSchema(t)
		return nodeDesc{Type: t, Label: label, Category: cat, Description: desc, Schema: schema}
	}

	return map[string]interface{}{
		"triggers": []nodeDesc{
			mkNode("trigger.manual", "Manual Trigger", "triggers", "Start a workflow run manually"),
			mkNode("trigger.schedule", "Schedule", "triggers", "Run workflow on a cron or interval schedule"),
			mkNode("trigger.webhook", "Webhook", "triggers", "Start workflow on incoming HTTP request"),
		},
		"control": []nodeDesc{
			mkNode("core.if", "If", "control", "Branch execution based on a condition"),
			mkNode("core.switch", "Switch", "control", "Route execution to one of multiple branches"),
			mkNode("core.merge", "Merge", "control", "Merge multiple input branches into one"),
			mkNode("core.split_in_batches", "Split In Batches", "control", "Process items in fixed-size batches"),
			mkNode("core.wait", "Wait", "control", "Pause execution for a specified duration"),
			mkNode("core.stop_error", "Stop And Error", "control", "Halt workflow with an error message"),
			mkNode("core.set", "Set", "control", "Set or transform data fields"),
			mkNode("core.code", "Code", "control", "Execute arbitrary JavaScript code"),
			mkNode("core.filter", "Filter", "control", "Keep only items matching a condition"),
			mkNode("core.sort", "Sort", "control", "Sort items by one or more fields"),
			mkNode("core.limit", "Limit", "control", "Limit the number of output items"),
			mkNode("core.remove_duplicates", "Remove Duplicates", "control", "Deduplicate items by key"),
			mkNode("core.compare_datasets", "Compare Datasets", "control", "Compare two datasets and output differences"),
			mkNode("core.aggregate", "Aggregate", "control", "Aggregate multiple items into one"),
		},
		"data": []nodeDesc{
			mkNode("data.datetime", "Date & Time", "data", "Parse, format, and manipulate date/time values"),
			mkNode("data.crypto", "Crypto", "data", "Hash, encrypt, or sign data"),
			mkNode("data.html", "HTML", "data", "Parse or generate HTML"),
			mkNode("data.xml", "XML", "data", "Parse or generate XML"),
			mkNode("data.markdown", "Markdown", "data", "Convert Markdown to HTML and vice-versa"),
			mkNode("data.spreadsheet", "Spreadsheet", "data", "Read or write spreadsheet data"),
			mkNode("data.compression", "Compression", "data", "Compress or decompress files"),
			mkNode("data.write_binary_file", "Write Binary File", "data", "Write binary data to disk"),
		},
		"http": []nodeDesc{
			mkNode("http.request", "HTTP Request", "http", "Make an HTTP/S request"),
			mkNode("http.ftp", "FTP", "http", "Transfer files via FTP/SFTP"),
			mkNode("http.ssh", "SSH", "http", "Execute commands over SSH"),
		},
		"system": []nodeDesc{
			mkNode("system.execute_command", "Execute Command", "system", "Run a shell command on the host"),
			mkNode("system.rss_read", "RSS Read", "system", "Fetch and parse an RSS/Atom feed"),
		},
		"db": []nodeDesc{
			mkNode("db.mysql", "MySQL", "db", "Query a MySQL/MariaDB database"),
			mkNode("db.postgres", "Postgres", "db", "Query a PostgreSQL database"),
			mkNode("db.mongodb", "MongoDB", "db", "Interact with a MongoDB collection"),
			mkNode("db.redis", "Redis", "db", "Read/write keys in a Redis store"),
		},
		"comm": []nodeDesc{
			mkNode("comm.email_send", "Send Email", "comm", "Send an email via SMTP"),
			mkNode("comm.email_read", "Read Email", "comm", "Read emails via IMAP"),
			mkNode("comm.slack", "Slack", "comm", "Send or read Slack messages"),
			mkNode("comm.telegram", "Telegram", "comm", "Send or receive Telegram messages"),
			mkNode("comm.discord", "Discord", "comm", "Send messages to a Discord channel"),
			mkNode("comm.twilio", "Twilio", "comm", "Send SMS or make calls via Twilio"),
			mkNode("comm.whatsapp", "WhatsApp", "comm", "Send WhatsApp messages"),
		},
		"service": []nodeDesc{
			mkNode("service.github", "GitHub", "service", "Interact with GitHub repositories and issues"),
			mkNode("service.airtable", "Airtable", "service", "Read/write Airtable bases"),
			mkNode("service.notion", "Notion", "service", "Read/write Notion pages and databases"),
			mkNode("service.jira", "Jira", "service", "Manage Jira issues and projects"),
			mkNode("service.linear", "Linear", "service", "Manage Linear issues and cycles"),
			mkNode("service.asana", "Asana", "service", "Manage Asana tasks and projects"),
			mkNode("service.stripe", "Stripe", "service", "Interact with Stripe payments"),
			mkNode("service.shopify", "Shopify", "service", "Manage Shopify orders and products"),
			mkNode("service.salesforce", "Salesforce", "service", "Read/write Salesforce records"),
			mkNode("service.hubspot", "HubSpot", "service", "Manage HubSpot CRM contacts and deals"),
			mkNode("service.google_sheets", "Google Sheets", "service", "Read/write Google Sheets"),
			mkNode("service.gmail", "Gmail", "service", "Send and read Gmail messages"),
			mkNode("service.google_drive", "Google Drive", "service", "Manage Google Drive files"),
			mkNode("service.huggingface", "HuggingFace", "service", "Generate images or text via HuggingFace Inference API"),
			mkNode("service.openrouter", "OpenRouter", "service", "Generate text or images via OpenRouter AI API"),
		},
		"ai": []nodeDesc{
			mkNode("ai.chat", "AI Chat", "ai", "Send a prompt to an AI model and get a response"),
			mkNode("ai.extract", "AI Extract", "ai", "Extract structured data from text using AI"),
			mkNode("ai.classify", "AI Classify", "ai", "Classify items into categories using AI"),
			mkNode("ai.transform", "AI Transform", "ai", "Transform text content using AI"),
			mkNode("ai.embed", "AI Embed", "ai", "Generate embeddings for text content"),
			mkNode("ai.agent", "AI Agent", "ai", "Autonomous AI agent that works toward a goal"),
			mkNode("ai.read_page", "Read Page", "ai", "Crawl a webpage and return clean readable content (markdown, links, images)"),
			mkNode("ai.extract_page", "Extract Page", "ai", "Extract specific fields from a webpage using AI or CSS selectors"),
		},
		"browser": []nodeDesc{
			// Instagram
			mkNode("instagram.find_by_keyword", "Instagram: Find By Keyword", "browser", "Search Instagram users or posts by keyword"),
			mkNode("instagram.export_followers", "Instagram: Export Followers", "browser", "Export a profile's follower list"),
			mkNode("instagram.scrape_profile_info", "Instagram: Scrape Profile Info", "browser", "Collect profile metadata"),
			mkNode("instagram.engage_with_posts", "Instagram: Engage With Posts", "browser", "Like/comment on matched posts"),
			mkNode("instagram.send_dms", "Instagram: Send DMs", "browser", "Send direct messages"),
			mkNode("instagram.auto_reply_dms", "Instagram: Auto Reply DMs", "browser", "Automatically reply to incoming DMs"),
			mkNode("instagram.publish_post", "Instagram: Publish Post", "browser", "Publish a photo or reel"),
			mkNode("instagram.like_posts", "Instagram: Like Posts", "browser", "Like a list of posts"),
			mkNode("instagram.comment_on_posts", "Instagram: Comment On Posts", "browser", "Comment on a list of posts"),
			mkNode("instagram.like_comments_on_posts", "Instagram: Like Comments On Posts", "browser", "Like comments on posts"),
			mkNode("instagram.extract_post_data", "Instagram: Extract Post Data", "browser", "Extract structured data from posts"),
			mkNode("instagram.follow_users", "Instagram: Follow Users", "browser", "Follow a list of users"),
			mkNode("instagram.unfollow_users", "Instagram: Unfollow Users", "browser", "Unfollow a list of users"),
			mkNode("instagram.watch_stories", "Instagram: Watch Stories", "browser", "View stories for a list of users"),
			mkNode("instagram.engage_user_posts", "Instagram: Engage User Posts", "browser", "Engage with a specific user's posts"),
			mkNode("instagram.list_post_comments", "Instagram: List Post Comments", "browser", "List comments on a post"),
			mkNode("instagram.list_user_posts", "Instagram: List User Posts", "browser", "List posts from a user profile"),
			mkNode("instagram.reply_to_comments", "Instagram: Reply To Comments", "browser", "Reply to comments on posts"),
			// LinkedIn
			mkNode("linkedin.find_by_keyword", "LinkedIn: Find By Keyword", "browser", "Search LinkedIn profiles by keyword"),
			mkNode("linkedin.export_followers", "LinkedIn: Export Followers", "browser", "Export a profile's connections/followers"),
			mkNode("linkedin.scrape_profile_info", "LinkedIn: Scrape Profile Info", "browser", "Collect LinkedIn profile metadata"),
			mkNode("linkedin.engage_with_posts", "LinkedIn: Engage With Posts", "browser", "Like/comment on LinkedIn posts"),
			mkNode("linkedin.send_dms", "LinkedIn: Send DMs", "browser", "Send LinkedIn direct messages"),
			mkNode("linkedin.auto_reply_dms", "LinkedIn: Auto Reply DMs", "browser", "Automatically reply to LinkedIn messages"),
			mkNode("linkedin.publish_post", "LinkedIn: Publish Post", "browser", "Publish a LinkedIn post"),
			mkNode("linkedin.comment_on_posts", "LinkedIn: Comment On Posts", "browser", "Comment on LinkedIn posts"),
			mkNode("linkedin.like_comments", "LinkedIn: Like Comments", "browser", "Like comments on LinkedIn posts"),
			mkNode("linkedin.like_posts", "LinkedIn: Like Posts", "browser", "Like LinkedIn posts"),
			mkNode("linkedin.list_post_comments", "LinkedIn: List Post Comments", "browser", "List comments on a LinkedIn post"),
			mkNode("linkedin.list_user_posts", "LinkedIn: List User Posts", "browser", "List posts from a LinkedIn profile"),
			// X (Twitter)
			mkNode("x.find_by_keyword", "X: Find By Keyword", "browser", "Search X/Twitter by keyword"),
			mkNode("x.export_followers", "X: Export Followers", "browser", "Export a profile's followers on X"),
			mkNode("x.scrape_profile_info", "X: Scrape Profile Info", "browser", "Collect X profile metadata"),
			mkNode("x.engage_with_posts", "X: Engage With Posts", "browser", "Like/reply to X posts"),
			mkNode("x.send_dms", "X: Send DMs", "browser", "Send X direct messages"),
			mkNode("x.auto_reply_dms", "X: Auto Reply DMs", "browser", "Automatically reply to X DMs"),
			mkNode("x.publish_post", "X: Publish Post", "browser", "Publish a post on X"),
			// TikTok
			mkNode("tiktok.find_by_keyword", "TikTok: Find By Keyword", "browser", "Search TikTok by keyword"),
			mkNode("tiktok.export_followers", "TikTok: Export Followers", "browser", "Export a TikTok profile's followers"),
			mkNode("tiktok.scrape_profile_info", "TikTok: Scrape Profile Info", "browser", "Collect TikTok profile metadata"),
			mkNode("tiktok.engage_with_posts", "TikTok: Engage With Posts", "browser", "Like/comment on TikTok posts"),
			mkNode("tiktok.send_dms", "TikTok: Send DMs", "browser", "Send TikTok direct messages"),
			mkNode("tiktok.auto_reply_dms", "TikTok: Auto Reply DMs", "browser", "Automatically reply to TikTok DMs"),
			mkNode("tiktok.publish_post", "TikTok: Publish Post", "browser", "Publish a TikTok video"),
			mkNode("tiktok.comment_on_video", "TikTok: Comment On Video", "browser", "Comment on a TikTok video"),
			mkNode("tiktok.duet_video", "TikTok: Duet Video", "browser", "Create a duet with a TikTok video"),
			mkNode("tiktok.follow_user", "TikTok: Follow User", "browser", "Follow a TikTok user"),
			mkNode("tiktok.like_comment", "TikTok: Like Comment", "browser", "Like a comment on a TikTok video"),
			mkNode("tiktok.like_video", "TikTok: Like Video", "browser", "Like a TikTok video"),
			mkNode("tiktok.list_user_videos", "TikTok: List User Videos", "browser", "List videos from a TikTok profile"),
			mkNode("tiktok.list_video_comments", "TikTok: List Video Comments", "browser", "List comments on a TikTok video"),
			mkNode("tiktok.share_video", "TikTok: Share Video", "browser", "Share a TikTok video"),
			mkNode("tiktok.stitch_video", "TikTok: Stitch Video", "browser", "Create a stitch with a TikTok video"),
		},
		"people": []nodeDesc{
			mkNode("people.save", "Save to People", "people", "Upsert items into the People tab"),
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Node runner — execute any node type directly
// ─────────────────────────────────────────────────────────────────────────────

// NodeRunRequest is the payload sent by the frontend to run a node.
type NodeRunRequest struct {
	NodeType string                   `json:"node_type"`
	Config   map[string]interface{}   `json:"config"`
	Items    []map[string]interface{} `json:"items"` // each element is a JSON object (the item's .json field)
}

// NodeRunResult is returned after running a node.
type NodeRunResult struct {
	Outputs []NodeRunOutput `json:"outputs"`
	Error   string          `json:"error,omitempty"`
	DurationMs int64        `json:"duration_ms"`
}

// NodeRunOutput is one output handle's items.
type NodeRunOutput struct {
	Handle string                   `json:"handle"`
	Items  []map[string]interface{} `json:"items"`
}

// RunNode executes any registered node type directly via the CLI subprocess.
// Config and input items are passed as JSON; results are returned as structured data.
// legacyNodeTypes maps old short type names to their new prefixed equivalents.
var legacyNodeTypes = map[string]string{
	"if": "core.if", "switch": "core.switch", "merge": "core.merge",
	"split_in_batches": "core.split_in_batches", "wait": "core.wait",
	"stop_error": "core.stop_error", "set": "core.set", "code": "core.code",
	"filter": "core.filter", "sort": "core.sort", "limit": "core.limit",
	"remove_duplicates": "core.remove_duplicates", "compare_datasets": "core.compare_datasets",
	"aggregate": "core.aggregate",
	"datetime": "data.datetime", "crypto": "data.crypto", "html": "data.html",
	"xml": "data.xml", "markdown": "data.markdown", "spreadsheet": "data.spreadsheet",
	"compression": "data.compression", "write_binary_file": "data.write_binary_file",
	"mysql": "db.mysql", "postgres": "db.postgres", "mongodb": "db.mongodb", "redis": "db.redis",
	"email_send": "comm.email_send", "email_read": "comm.email_read",
	"slack": "comm.slack", "telegram": "comm.telegram", "discord": "comm.discord",
	"twilio": "comm.twilio", "whatsapp": "comm.whatsapp",
	"github": "service.github", "airtable": "service.airtable", "notion": "service.notion",
	"jira": "service.jira", "linear": "service.linear", "asana": "service.asana",
	"stripe": "service.stripe", "shopify": "service.shopify", "salesforce": "service.salesforce",
	"hubspot": "service.hubspot", "google_sheets": "service.google_sheets",
	"gmail": "service.gmail", "google_drive": "service.google_drive",
}

// isBrowserNodeType returns true for platform.action social/browser node types.
func isBrowserNodeType(t string) bool {
	return strings.HasPrefix(t, "instagram.") || strings.HasPrefix(t, "linkedin.") ||
		strings.HasPrefix(t, "x.") || strings.HasPrefix(t, "tiktok.")
}

func (a *App) RunNode(req NodeRunRequest) NodeRunResult {
	// Normalize legacy short type names to prefixed names.
	if mapped, ok := legacyNodeTypes[req.NodeType]; ok {
		req.NodeType = mapped
	}

	// Resolve credential_id → merge connection credentials into config.
	// Uses getResourceCredentialData which handles token refresh for expired OAuth tokens.
	if credID, ok := req.Config["credential_id"].(string); ok && credID != "" && a.connMgr != nil {
		credData, err := a.getResourceCredentialData(context.Background(), credID)
		if err != nil {
			// Fallback: derive platform from node type and try by platform name.
			platform := nodeTypeToPlatform(req.NodeType)
			if platform != "" {
				credData, err = a.getResourceCredentialData(context.Background(), platform)
			}
			if err != nil {
				return NodeRunResult{Error: fmt.Sprintf("resolve credential %s: %v", credID, err)}
			}
		}
		// Merge connection data fields into config (connection credentials take precedence).
		for k, v := range credData {
			req.Config[k] = v
		}
		delete(req.Config, "credential_id")
	}

	// Browser/social nodes run in-process (they need a live browser session).
	if isBrowserNodeType(req.NodeType) {
		return a.runBrowserNode(req)
	}

	monoesBin, err := findMonoesBinary()
	if err != nil {
		return NodeRunResult{Error: err.Error()}
	}

	// Build --config JSON
	configBytes, err := json.Marshal(req.Config)
	if err != nil {
		return NodeRunResult{Error: "invalid config: " + err.Error()}
	}

	// Build --input JSON array
	items := req.Items
	if len(items) == 0 {
		items = []map[string]interface{}{{"json": map[string]interface{}{}}}
	}
	inputItems := make([]map[string]interface{}, len(items))
	for i, it := range items {
		inputItems[i] = map[string]interface{}{"json": it}
	}
	inputBytes, err := json.Marshal(inputItems)
	if err != nil {
		return NodeRunResult{Error: "invalid input: " + err.Error()}
	}

	start := time.Now()
	cmd := exec.Command(monoesBin,
		"node", "run", req.NodeType,
		"--config", string(configBytes),
		"--input", string(inputBytes),
		"--output", "json",
	)
	out, runErr := cmd.Output()
	elapsed := time.Since(start).Milliseconds()

	if runErr != nil {
		// cmd.Output captures stderr only on error via *exec.ExitError
		msg := runErr.Error()
		if exitErr, ok := runErr.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			// strip "exit status N" suffix from actual message
			msg = strings.TrimSpace(string(exitErr.Stderr))
		}
		return NodeRunResult{Error: msg, DurationMs: elapsed}
	}

	// Parse JSON output: map[handle][]Item
	var raw map[string][]struct {
		JSON map[string]interface{} `json:"json"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return NodeRunResult{Error: "failed to parse output: " + err.Error(), DurationMs: elapsed}
	}

	var outputs []NodeRunOutput
	for handle, rawItems := range raw {
		flat := make([]map[string]interface{}, len(rawItems))
		for i, ri := range rawItems {
			flat[i] = ri.JSON
		}
		outputs = append(outputs, NodeRunOutput{Handle: handle, Items: flat})
	}
	// Sort outputs by handle for deterministic order
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].Handle < outputs[j].Handle })

	return NodeRunResult{Outputs: outputs, DurationMs: elapsed}
}

func nopLogger() zerolog.Logger { return zerolog.New(io.Discard) }

// runBrowserNode executes a browser/social node in-process using the local browser.
// It launches a browser, restores session cookies from the DB, and runs the action.
func (a *App) runBrowserNode(req NodeRunRequest) NodeRunResult {
	start := time.Now()

	// Parse "platform.action_type" → platform, actionType
	parts := strings.SplitN(req.NodeType, ".", 2)
	if len(parts) != 2 {
		return NodeRunResult{Error: fmt.Sprintf("invalid browser node type: %s", req.NodeType)}
	}
	platform, actionType := parts[0], parts[1]

	// 1. Launch browser page with anti-detection.
	launchURL, err := launcher.New().
		Headless(false).
		Set("disable-blink-features", "AutomationControlled").
		Launch()
	if err != nil {
		return NodeRunResult{Error: "failed to launch browser: " + err.Error(), DurationMs: time.Since(start).Milliseconds()}
	}
	browser := rod.New().ControlURL(launchURL)
	if err := browser.Connect(); err != nil {
		return NodeRunResult{Error: "failed to connect browser: " + err.Error(), DurationMs: time.Since(start).Milliseconds()}
	}
	defer browser.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return NodeRunResult{Error: "failed to create page: " + err.Error(), DurationMs: time.Since(start).Milliseconds()}
	}

	// 2. Restore session cookies from DB.
	if a.db != nil {
		var cookiesJSON string
		qErr := a.db.QueryRow(
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

	// 3. Get bot adapter for call_bot_method steps.
	var botAdapter action.BotAdapter
	if factory, ok := bot.PlatformRegistry[strings.ToUpper(platform)]; ok {
		adapter := factory()
		if ba, ok := adapter.(action.BotAdapter); ok {
			botAdapter = ba
		}
	}

	// 4. Build StorageAction from config.
	sa := &action.StorageAction{
		ID:             uuid.New().String(),
		Type:           actionType,
		TargetPlatform: platform,
	}
	if msg, ok := req.Config["message"].(string); ok {
		sa.ContentMessage = msg
	}
	if kw, ok := req.Config["keywords"].(string); ok {
		sa.Keywords = kw
	}
	if targetsRaw, ok := req.Config["targets"]; ok {
		if targets, ok := targetsRaw.([]interface{}); ok {
			sa.Params = map[string]interface{}{"targets": targets}
		}
	}
	if sa.Params == nil {
		sa.Params = make(map[string]interface{})
	}
	for k, v := range req.Config {
		if k != "username" && k != "targets" && k != "message" && k != "keywords" {
			sa.Params[k] = v
		}
	}

	// 5. Execute the action.
	executor := action.NewActionExecutor(
		a.ctx,
		page,
		nil, // db storage - not needed for workflow execution
		a.cfgMgr,
		nil, // events channel
		botAdapter,
		nopLogger(),
	)

	// Seed targets if provided.
	if targetsRaw, ok := req.Config["targets"]; ok {
		if targets, ok := targetsRaw.([]interface{}); ok && len(targets) > 0 {
			executor.SetVariable("selectedListItems", targets)
		}
	}

	result, err := executor.Execute(sa)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return NodeRunResult{Error: err.Error(), DurationMs: elapsed}
	}

	// 6. For profile-scraping actions, auto-save results to the people table.
	if strings.HasSuffix(actionType, "scrape_profile_info") && a.db != nil && len(result.ExtractedItems) > 0 {
		_ = a.saveProfilesToPeople(result.ExtractedItems, strings.ToUpper(platform))
	}

	// 7. Convert results to NodeRunOutput.
	return NodeRunResult{
		Outputs:    []NodeRunOutput{{Handle: "main", Items: result.ExtractedItems}},
		DurationMs: elapsed,
	}
}

// saveProfilesToPeople upserts scraped profile items into the people table.
func (a *App) saveProfilesToPeople(items []map[string]interface{}, defaultPlatform string) error {
	now := time.Now().UTC()
	for _, data := range items {
		platformRaw, _ := data["platform"].(string)
		if platformRaw == "" {
			platformRaw = defaultPlatform
		}
		platformUpper := strings.ToUpper(platformRaw)

		profileURL := firstStringFromMap(data, "profile_url", "url", "href")
		username := ""
		if profileURL != "" {
			if factory, ok := bot.PlatformRegistry[platformUpper]; ok {
				username = factory().ExtractUsername(profileURL)
			}
			if username == "" {
				parts := strings.Split(strings.Trim(profileURL, "/"), "/")
				if len(parts) > 0 {
					username = strings.TrimPrefix(parts[len(parts)-1], "@")
				}
			}
		}
		if username == "" {
			continue
		}

		fullName, _ := data["full_name"].(string)
		imageURL, _ := data["image_url"].(string)
		website, _ := data["website"].(string)
		introduction, _ := data["introduction"].(string)
		isVerified, _ := data["is_verified"].(bool)
		jobTitle := firstStringFromMap(data, "job_title", "position", "headline")

		followerInt := int64ToNullable(parseAbbrevInt(mapStrVal(data, "follower_count", "followers_count")))
		followingInt := int64ToNullable(parseAbbrevInt(mapStrVal(data, "following_count")))
		contentInt := int64ToNullable(parseAbbrevInt(mapStrVal(data, "content_count")))

		_, err := a.db.Exec(
			`INSERT INTO people (id, platform_username, platform, full_name, image_url,
			        contact_details, website, content_count, follower_count,
			        following_count, introduction, is_verified, category, job_title,
			        profile_url, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT(platform_username, platform)
			 DO UPDATE SET
			   full_name       = COALESCE(excluded.full_name,       people.full_name),
			   image_url       = COALESCE(excluded.image_url,       people.image_url),
			   profile_url     = COALESCE(excluded.profile_url,     people.profile_url),
			   website         = COALESCE(excluded.website,         people.website),
			   content_count   = COALESCE(excluded.content_count,   people.content_count),
			   follower_count  = COALESCE(excluded.follower_count,  people.follower_count),
			   following_count = COALESCE(excluded.following_count, people.following_count),
			   introduction    = COALESCE(excluded.introduction,    people.introduction),
			   is_verified     = COALESCE(excluded.is_verified,     people.is_verified),
			   job_title       = COALESCE(excluded.job_title,       people.job_title),
			   updated_at      = excluded.updated_at`,
			uuid.New().String(), username, platformUpper,
			nullStr(fullName), nullStr(imageURL), nil,
			nullStr(website), contentInt, followerInt, followingInt,
			nullStr(introduction), isVerified, nil, nullStr(jobTitle),
			nullStr(profileURL), now, now,
		)
		if err != nil {
			return fmt.Errorf("saveProfilesToPeople %s/%s: %w", platformUpper, username, err)
		}
	}
	return nil
}

func firstStringFromMap(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func mapStrVal(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return fmt.Sprintf("%d", int64(v))
		case int64:
			return fmt.Sprintf("%d", v)
		}
	}
	return ""
}

func parseAbbrevInt(s string) int64 {
	if s == "" {
		return 0
	}
	// Strip trailing word suffixes like "followers", "posts"
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0
	}
	s = parts[0]
	s = strings.ToUpper(strings.TrimSpace(s))
	multiplier := int64(1)
	if strings.HasSuffix(s, "K") {
		multiplier = 1_000
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1_000_000
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1_000_000_000
		s = s[:len(s)-1]
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return int64(f * float64(multiplier))
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func int64ToNullable(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// ─────────────────────────────────────────────────────────────────────────────
// Connections
// ─────────────────────────────────────────────────────────────────────────────

// CredentialOption is a lightweight connection summary used to populate
// credential dropdowns in the workflow node inspector.
type CredentialOption struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Platform string `json:"platform"`
	Method   string `json:"method"`
}

// ListCredentialsForNode returns credential options relevant to a given node type.
// Social platform nodes (action.instagram.*, action.linkedin.*, etc.) get browser
// method credentials; API service nodes get their matching platform's connections.
func (a *App) ListCredentialsForNode(nodeType string) []CredentialOption {
	if a.db == nil {
		return []CredentialOption{}
	}
	store := connections.NewStore(a.db)
	var platform string

	// Detect social platform from node type prefix (e.g. "action.instagram.publish_post")
	socialPlatforms := []string{"instagram", "linkedin", "tiktok", "x", "twitter"}
	lnodeType := strings.ToLower(nodeType)
	for _, sp := range socialPlatforms {
		if strings.Contains(lnodeType, sp) {
			platform = sp
			break
		}
	}

	// Service nodes: map by known service identifiers
	if platform == "" {
		serviceMap := map[string]string{
			"openrouter":    "openrouter",
			"huggingface":   "huggingface",
			"google_sheets": "google",
			"gmail":         "google",
			"slack":         "slack",
		}
		for key, pid := range serviceMap {
			if strings.Contains(lnodeType, key) {
				platform = pid
				break
			}
		}
	}

	var conns []connections.Connection
	var err error
	if platform != "" {
		conns, err = store.ListByPlatform(a.ctx, platform)
	} else {
		conns, err = store.ListAll(a.ctx)
	}
	if err != nil || conns == nil {
		return []CredentialOption{}
	}
	opts := make([]CredentialOption, 0, len(conns))
	for _, c := range conns {
		opts = append(opts, CredentialOption{
			ID:       c.ID,
			Label:    c.Label,
			Platform: c.Platform,
			Method:   string(c.Method),
		})
	}
	return opts
}

// ListConnections returns all saved connections, filtered by platform if non-empty.
func (a *App) ListConnections(platform string) []connections.Connection {
	if a.connMgr == nil {
		return []connections.Connection{}
	}
	result, err := a.connMgr.List(a.ctx, platform)
	if err != nil {
		return []connections.Connection{}
	}
	if result == nil {
		result = []connections.Connection{}
	}
	return result
}

// PlatformInfo is a frontend-safe representation of a platform (no OAuth secrets).
type PlatformInfo struct {
	ID         string                                       `json:"id"`
	Name       string                                       `json:"name"`
	Category   string                                       `json:"category"`
	ConnectVia string                                       `json:"connectVia"`
	Methods    []string                                     `json:"methods"`
	Fields     map[string][]connections.CredentialField     `json:"fields"`
	IconEmoji  string                                       `json:"iconEmoji"`
}

func toPlatformInfo(p connections.PlatformDef) PlatformInfo {
	methods := make([]string, len(p.Methods))
	for i, m := range p.Methods {
		methods[i] = string(m)
	}
	fields := make(map[string][]connections.CredentialField)
	for method, cfields := range p.Fields {
		fields[string(method)] = cfields
	}
	return PlatformInfo{
		ID:         p.ID,
		Name:       p.Name,
		Category:   p.Category,
		ConnectVia: p.ConnectVia,
		Methods:    methods,
		Fields:     fields,
		IconEmoji:  p.IconEmoji,
	}
}

// ListPlatformsJSON returns all platforms as a JSON string (bypasses Wails type serialization).
func (a *App) ListPlatformsJSON(connectVia string) string {
	var platforms []connections.PlatformDef
	if connectVia == "" {
		platforms = connections.All()
	} else {
		platforms = connections.ByConnectVia(connectVia)
	}
	result := make([]PlatformInfo, len(platforms))
	for i, p := range platforms {
		result[i] = toPlatformInfo(p)
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// TestConnection re-validates a connection by ID.
// For OAuth connections it attempts a silent token refresh first.
// For browser sessions (social platforms), it checks session expiry and cookie presence.
func (a *App) TestConnection(id string) string {
	if a.connMgr == nil && a.db == nil {
		return "error: manager not initialized"
	}

	// First try the connections table (OAuth/API key connections).
	if a.connMgr != nil {
		if conn, err := a.connMgr.Get(a.ctx, id); err == nil && conn != nil {
			// OAuth: attempt silent token refresh before testing.
			if conn.Method == "oauth" {
				if _, refreshErr := a.getResourceCredentialData(a.ctx, id); refreshErr != nil {
					fmt.Printf("token refresh attempted: %v\n", refreshErr)
				}
			}
			if err := a.connMgr.Test(a.ctx, id); err != nil {
				return fmt.Sprintf("error: %v", err)
			}
			return "ok"
		}
	}

	// Fallback: check crawler_sessions (browser sessions for social platforms).
	// The UI passes integer session IDs for social platforms.
	if a.db != nil {
		var platform, cookiesJSON, expiry string
		err := a.db.QueryRow(
			`SELECT platform, cookies_json, expiry FROM crawler_sessions WHERE id = ?`, id,
		).Scan(&platform, &cookiesJSON, &expiry)
		if err == nil {
			// Check expiry
			if exp, pErr := time.Parse("2006-01-02 15:04:05", expiry); pErr == nil {
				if time.Now().After(exp) {
					return "error: session expired — please log in again via the browser"
				}
			} else if exp2, pErr2 := time.Parse(time.RFC3339, expiry); pErr2 == nil {
				if time.Now().After(exp2) {
					return "error: session expired — please log in again via the browser"
				}
			}
			// Check cookies present
			if cookiesJSON == "" || cookiesJSON == "[]" || cookiesJSON == "null" {
				return "error: no session cookies stored — please log in again"
			}
			return "ok"
		}
	}

	// Also try looking up by platform name (fallback for credential_id = platform string).
	if a.connMgr != nil {
		platform := nodeTypeToPlatform(id)
		if conns, err := a.connMgr.List(a.ctx, platform); err == nil && len(conns) > 0 {
			for _, c := range conns {
				if c.Status == "active" {
					return "ok"
				}
			}
		}
	}

	return "error: connection not found"
}

// RemoveConnection deletes a connection by ID.
func (a *App) RemoveConnection(id string) string {
	if a.connMgr == nil {
		return "error: manager not initialized"
	}
	if err := a.connMgr.Remove(a.ctx, id); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return "ok"
}

// GetConnectionsForPlatform returns connections filtered by platform ID.
func (a *App) GetConnectionsForPlatform(platformID string) []connections.Connection {
	return a.ListConnections(platformID)
}

// ensureOAuthCredsTable creates the platform_oauth_credentials table if it doesn't exist.
func (a *App) ensureOAuthCredsTable() error {
	_, err := a.db.Exec(`CREATE TABLE IF NOT EXISTS platform_oauth_credentials (
		platform      TEXT PRIMARY KEY,
		client_id     TEXT NOT NULL,
		client_secret TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	)`)
	return err
}

// GetOAuthCredentials returns the stored OAuth client_id and client_secret for a platform as JSON.
// Returns JSON {"clientID":"...","clientSecret":"..."} or "" if not set.
func (a *App) GetOAuthCredentials(platformID string) string {
	if a.db == nil {
		return ""
	}
	_ = a.ensureOAuthCredsTable()
	var clientID, clientSecret string
	err := a.db.QueryRow(
		`SELECT client_id, client_secret FROM platform_oauth_credentials WHERE platform = ?`, platformID,
	).Scan(&clientID, &clientSecret)
	if err != nil {
		return ""
	}
	b, _ := json.Marshal(map[string]string{"clientID": clientID, "clientSecret": clientSecret})
	return string(b)
}

// SetOAuthCredentials saves OAuth client_id and client_secret for a platform.
func (a *App) SetOAuthCredentials(platformID, clientID, clientSecret string) string {
	if a.db == nil {
		return "error: db not available"
	}
	if clientID == "" || clientSecret == "" {
		return "error: clientID and clientSecret are required"
	}
	_ = a.ensureOAuthCredsTable()
	_, err := a.db.Exec(
		`INSERT OR REPLACE INTO platform_oauth_credentials (platform, client_id, client_secret, updated_at)
		 VALUES (?, ?, ?, ?)`,
		platformID, clientID, clientSecret, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return "ok"
}

// ConnectPlatformOAuth starts an OAuth flow in a background goroutine.
// Emits "conn:progress" events with {platform, message, kind} and a final
// "conn:done" event with {platform, success, accountID?, error?}.
// Returns "started" immediately, or "error: ..." if preconditions fail.
func (a *App) ConnectPlatformOAuth(platformID string) string {
	if a.connMgr == nil {
		return "error: manager not initialized"
	}
	p, ok := connections.Get(platformID)
	if !ok {
		return fmt.Sprintf("error: unknown platform %q", platformID)
	}
	if p.OAuth == nil {
		return "error: platform does not support OAuth"
	}

	go func() {
		emit := func(msg, kind string) {
			runtime.EventsEmit(a.ctx, "conn:progress", map[string]interface{}{
				"platform": platformID,
				"message":  msg,
				"kind":     kind,
			})
		}

		// Resolve DB-stored OAuth credentials and pass them directly.
		var oauthClientID, oauthClientSecret string
		if credsJSON := a.GetOAuthCredentials(platformID); credsJSON != "" {
			var creds map[string]string
			if json.Unmarshal([]byte(credsJSON), &creds) == nil {
				oauthClientID = creds["clientID"]
				oauthClientSecret = creds["clientSecret"]
			}
		}

		conn, err := a.connMgr.ConnectOAuthWithProgress(a.ctx, platformID, emit, oauthClientID, oauthClientSecret)
		if err != nil {
			runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{
				"platform": platformID,
				"success":  false,
				"error":    err.Error(),
			})
			return
		}

		runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{
			"platform":  platformID,
			"success":   true,
			"accountID": conn.AccountID,
		})
	}()

	return "started"
}

// findSystemChrome returns the path to the user's real Chrome browser.
func findSystemChrome() string {
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// LoginSocial opens a visible browser window for the user to log in to a social platform.
// Runs the login flow asynchronously, emitting conn:progress and conn:done events.
// Returns "started" immediately or "error: ..." if the platform is unknown.
func (a *App) LoginSocial(platform string) string {
	pid := strings.ToLower(platform)
	factory, ok := bot.PlatformRegistry[strings.ToUpper(platform)]
	if !ok {
		return fmt.Sprintf("error: unsupported platform %q", platform)
	}

	emit := func(msg, kind string) {
		runtime.EventsEmit(a.ctx, "conn:progress", map[string]interface{}{
			"platform": pid,
			"message":  msg,
			"kind":     kind,
		})
	}

	go func() {
		adapter := factory()

		// Use system Chrome to avoid bot detection on platforms like X.
		chromePath := findSystemChrome()
		home, _ := os.UserHomeDir()
		userDataDir := filepath.Join(home, ".monoes", "chrome-profile")
		u, launchErr := launcher.New().
			Leakless(false).
			Bin(chromePath).
			UserDataDir(userDataDir).
			Set("disable-blink-features", "AutomationControlled").
			Set("excludeSwitches", "enable-automation").
			Headless(false).
			Launch()
		if launchErr != nil {
			// Fallback to standalone Chromium.
			u, launchErr = launcher.New().Headless(false).
				Set("disable-blink-features", "AutomationControlled").
				Launch()
			if launchErr != nil {
				emit(fmt.Sprintf("Failed to launch browser: %v", launchErr), "error")
				runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": launchErr.Error()})
				return
			}
		}

		browser := rod.New().ControlURL(u)
		if err := browser.Connect(); err != nil {
			emit(fmt.Sprintf("Failed to connect browser: %v", err), "error")
			runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": err.Error()})
			return
		}
		defer browser.Close()

		page, err := browser.Page(proto.TargetCreateTarget{URL: adapter.LoginURL()})
		if err != nil {
			emit(fmt.Sprintf("Failed to open login page: %v", err), "error")
			runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": err.Error()})
			return
		}

		platformName := strings.ToUpper(pid[:1]) + pid[1:]
		emit("Browser opened — please log in to "+platformName+" in the window that appeared", "info")

		ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
		defer cancel()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				emit("Login timed out after 5 minutes", "error")
				runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": "timed out"})
				return
			case <-ticker.C:
				loggedIn, checkErr := adapter.IsLoggedIn(page)
				if checkErr != nil {
					continue
				}
				if loggedIn {
					emit("Login detected! Capturing session…", "info")
					cookies, cookieErr := page.Cookies(nil)
					if cookieErr != nil {
						emit(fmt.Sprintf("Failed to capture cookies: %v", cookieErr), "error")
						runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": cookieErr.Error()})
						return
					}
					cookiesJSON, marshalErr := json.Marshal(cookies)
					if marshalErr != nil {
						emit(fmt.Sprintf("Failed to encode cookies: %v", marshalErr), "error")
						runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": marshalErr.Error()})
						return
					}
					username := adapter.ExtractUsername(page.MustInfo().URL)
					if username == "" {
						username = "unknown"
					}
					expiry := time.Now().Add(30 * 24 * time.Hour)
					if a.db == nil {
						emit("Database not available", "error")
						runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": "db nil"})
						return
					}
					_, dbErr := a.db.Exec(
						`INSERT OR REPLACE INTO crawler_sessions (username, platform, cookies_json, expiry)
						 VALUES (?, ?, ?, ?)`,
						username, pid, string(cookiesJSON), expiry,
					)
					if dbErr != nil {
						emit(fmt.Sprintf("Failed to save session: %v", dbErr), "error")
						runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": false, "error": dbErr.Error()})
						return
					}
					// Mirror the browser session into the connections table so
					// workflow nodes can reference it via a stable credential_id.
					connStore := connections.NewStore(a.db)
					pName := strings.ToUpper(pid[:1]) + pid[1:]
					_ = connStore.Save(a.ctx, &connections.Connection{
						ID:        fmt.Sprintf("social:%s:%s", pid, username),
						Platform:  pid,
						Method:    connections.MethodBrowser,
						Label:     fmt.Sprintf("%s (@%s)", pName, username),
						AccountID: username,
						Data:      map[string]interface{}{"username": username},
						Status:    "active",
					})
					emit(fmt.Sprintf("Connected as %s", username), "success")
					runtime.EventsEmit(a.ctx, "conn:done", map[string]interface{}{"platform": pid, "success": true, "accountID": username})
					return
				}
			}
		}
	}()

	return "started"
}

// SaveConnectionDirect saves a connection directly from the UI with provided field values.
// fieldValuesJSON is a JSON object string (avoids Wails map serialization issues).
// Returns "ok:<id>" on success or "error: ..." on failure.
func (a *App) SaveConnectionDirect(platformID string, method string, fieldValuesJSON string) string {
	if a.connMgr == nil {
		return "error: manager not initialized"
	}
	p, ok := connections.Get(platformID)
	if !ok {
		return fmt.Sprintf("error: unknown platform %q", platformID)
	}
	var fieldValues map[string]interface{}
	if err := json.Unmarshal([]byte(fieldValuesJSON), &fieldValues); err != nil {
		return fmt.Sprintf("error: invalid field values JSON: %v", err)
	}
	now := time.Now().Format(time.RFC3339)
	conn := &connections.Connection{
		ID:        uuid.New().String(),
		Platform:  platformID,
		Method:    connections.AuthMethod(method),
		Label:     p.Name,
		Data:      fieldValues,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	// Validate the connection
	accountID, err := connections.ValidateConnection(a.ctx, conn)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	if accountID != "" {
		conn.AccountID = accountID
		conn.Label = fmt.Sprintf("%s – %s", p.Name, accountID)
	}
	// Save to DB
	store := connections.NewStore(a.db)
	if err := store.EnsureTable(a.ctx); err != nil {
		return fmt.Sprintf("error: table init: %v", err)
	}
	if err := store.Save(a.ctx, conn); err != nil {
		return fmt.Sprintf("error: save: %v", err)
	}
	return "ok:" + conn.ID
}

// ─────────────────────────────────────────────────────────────────────────────
// AI Providers
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) ListAIProviders() string {
	if a.aiStore == nil {
		return "[]"
	}
	providers, err := a.aiStore.ListProviders()
	if err != nil {
		return aiError(err)
	}
	b, _ := json.Marshal(providers)
	return string(b)
}

func (a *App) SaveAIProvider(providerJSON string) string {
	if a.aiStore == nil {
		return aiError(fmt.Errorf("ai store not initialized"))
	}
	var p ai.AIProvider
	if err := json.Unmarshal([]byte(providerJSON), &p); err != nil {
		return aiError(err)
	}
	if p.ID == "" {
		p.ID = newUUID()
	}
	if err := a.aiStore.SaveProvider(p); err != nil {
		return aiError(err)
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func (a *App) DeleteAIProvider(id string) string {
	if a.aiStore == nil {
		return aiError(fmt.Errorf("ai store not initialized"))
	}
	if err := a.aiStore.DeleteProvider(id); err != nil {
		return aiError(err)
	}
	return `{"ok":true}`
}

func (a *App) TestAIProvider(id string) string {
	if a.aiStore == nil {
		return aiError(fmt.Errorf("ai store not initialized"))
	}
	p, err := a.aiStore.GetProvider(id)
	if err != nil {
		return aiError(err)
	}
	client, err := ai.NewClient(p)
	if err != nil {
		return aiError(err)
	}
	model := p.DefaultModel
	if model == "" {
		def, ok := ai.GetProviderDef(p.ProviderID)
		if ok && len(def.Models) > 0 {
			model = def.Models[0].ID
		} else {
			model = "gpt-4o-mini"
		}
	}
	_, err = client.Complete(context.Background(), ai.CompletionRequest{
		Model:     model,
		Messages:  []ai.Message{{Role: ai.RoleUser, Content: "Say ok"}},
		MaxTokens: 5,
	})
	status := "active"
	if err != nil {
		status = "error"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_ = a.aiStore.UpdateProviderStatus(id, status, now)
	if err != nil {
		return fmt.Sprintf(`{"status":"error","error":%q}`, err.Error())
	}
	return `{"status":"active"}`
}

func (a *App) GetAIModels(providerID string) string {
	def, ok := ai.GetProviderDef(providerID)
	if !ok {
		return "[]"
	}
	b, _ := json.Marshal(def.Models)
	return string(b)
}

func (a *App) GetAIRegistry() string {
	b, _ := json.Marshal(ai.ProviderRegistry)
	return string(b)
}

func aiError(err error) string {
	return fmt.Sprintf(`{"error":%q}`, err.Error())
}

// ─────────────────────────────────────────────────────────────────────────────
// AI Chat
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) StreamAIChat(workflowID, message, providerID, model string) string {
	if a.chatService == nil {
		return aiError(fmt.Errorf("chat service not initialized"))
	}
	go func() {
		err := a.chatService.StreamChat(
			context.Background(),
			workflowID, message, providerID, model,
			func(chunk ai.StreamChunk) {
				runtime.EventsEmit(a.ctx, "ai:chunk", map[string]interface{}{
					"workflowID": workflowID,
					"content":    chunk.Content,
					"done":       chunk.Done,
				})
			},
			func(name, args, result string) {
				runtime.EventsEmit(a.ctx, "ai:tool", map[string]interface{}{
					"workflowID": workflowID,
					"tool":       name,
					"args":       args,
					"result":     result,
				})
			},
		)
		if err != nil {
			runtime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
				"workflowID": workflowID,
				"error":      err.Error(),
			})
		} else {
			// Signal streaming is complete.
			runtime.EventsEmit(a.ctx, "ai:chunk", map[string]interface{}{
				"workflowID": workflowID,
				"content":    "",
				"done":       true,
			})
		}
	}()
	return `{"ok":true}`
}

func (a *App) GetAIChatHistory(workflowID string) string {
	if a.chatService == nil {
		return "[]"
	}
	msgs, err := a.chatService.GetHistory(workflowID)
	if err != nil {
		return aiError(err)
	}
	b, _ := json.Marshal(msgs)
	return string(b)
}

func (a *App) ClearAIChatHistory(workflowID string) string {
	if a.chatService == nil {
		return aiError(fmt.Errorf("chat service not initialized"))
	}
	if err := a.chatService.ClearHistory(workflowID); err != nil {
		return aiError(err)
	}
	return `{"ok":true}`
}
