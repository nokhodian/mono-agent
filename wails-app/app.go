package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	_ "modernc.org/sqlite"
)

// App holds application state bound to the Wails runtime.
type App struct {
	ctx    context.Context
	db     *sql.DB
	dbPath string
	logs   []LogEntry
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
	}
	for _, q := range safeMigrations {
		_, _ = db.Exec(q)
	}

	a.emitLog("SYSTEM", "INFO", "Monoes Agent UI connected to "+a.dbPath)
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
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	State        string `json:"state"`
	Platform     string `json:"platform"`
	Keywords     string `json:"keywords"`
	ContentMsg   string `json:"content_message"`
	ReachedIndex int    `json:"reached_index"`
	ExecCount    int    `json:"exec_count"`
	TargetCount  int    `json:"target_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
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
	                             COALESCE(created_at_ts,''), COALESCE(updated_at_ts,'')
	                      FROM actions WHERE id = ?`, id)
	var act ActionInfo
	if row.Scan(&act.ID, &act.Title, &act.Type, &act.State, &act.Platform,
		&act.Keywords, &act.ContentMsg, &act.ReachedIndex, &act.ExecCount,
		&act.CreatedAt, &act.UpdatedAt) != nil {
		return nil
	}
	_ = a.db.QueryRow("SELECT COUNT(*) FROM action_targets WHERE action_id = ?", act.ID).Scan(&act.TargetCount)
	return &act
}

type CreateActionRequest struct {
	Title          string `json:"title"`
	Type           string `json:"type"`
	Platform       string `json:"platform"`
	Keywords       string `json:"keywords"`
	ContentMessage string `json:"content_message"`
}

func (a *App) CreateAction(req CreateActionRequest) (*ActionInfo, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	id := newUUID()
	now := time.Now()
	_, err := a.db.Exec(`INSERT INTO actions
	                      (id, created_at, title, type, state, target_platform, keywords, content_message, created_at_ts, updated_at_ts)
	                      VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?, ?, ?)`,
		id, now.Unix(), req.Title, strings.ToUpper(req.Type), strings.ToUpper(req.Platform),
		req.Keywords, req.ContentMessage, now.Format(time.RFC3339), now.Format(time.RFC3339))
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

func (a *App) GetPeople(platform, search string, limit, offset int) []PersonInfo {
	if a.db == nil {
		return nil
	}
	query := `SELECT id, platform_username, platform, COALESCE(full_name,''), COALESCE(image_url,''),
	                 COALESCE(profile_url,''), COALESCE(follower_count,''), following_count, is_verified,
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
		       COALESCE(follower_count,''), following_count, content_count, is_verified,
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
	_, err := os.Stat(p)
	return err == nil
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
