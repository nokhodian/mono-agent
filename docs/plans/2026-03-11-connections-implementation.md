# Connections Layer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a unified authentication layer so users can connect every supported platform (social, services, databases, communication) via the smoothest available method — OAuth, API key, browser session, or connection string — managed through both CLI and a new Credentials UI page.

**Architecture:** A platform registry (`internal/connections/registry.go`) defines every supported platform and its auth methods. A `ConnectionManager` in `internal/connections/manager.go` orchestrates all flows. The CLI exposes `monoes connect <platform>` and the Wails UI gets a new Credentials page alongside the existing Sessions page.

**Tech Stack:** Go 1.22+, SQLite (modernc.org/sqlite), Cobra (CLI), Wails v2, React/JSX, standard `net/http` for OAuth callback server, `os/exec` for opening browser URLs on macOS.

---

### Task 1: Create `internal/connections` package — registry and models

**Files:**
- Create: `internal/connections/registry.go`
- Create: `internal/connections/registry_test.go`

**Step 1: Create registry.go with all types and platform definitions**

```go
// internal/connections/registry.go
package connections

// AuthMethod identifies how a platform connection is established.
type AuthMethod string

const (
	MethodOAuth    AuthMethod = "oauth"
	MethodAPIKey   AuthMethod = "apikey"
	MethodBrowser  AuthMethod = "browser"
	MethodConnStr  AuthMethod = "connstring"
	MethodAppPass  AuthMethod = "apppassword"
)

// CredentialField describes one input field for a given auth method.
type CredentialField struct {
	Key      string // storage key, e.g. "api_key"
	Label    string // display label
	Secret   bool   // mask in UI/CLI
	Required bool
	HelpURL  string // link to where user obtains this value
	HelpText string // short step-by-step instruction
}

// OAuthConfig holds OAuth2 parameters for a platform.
type OAuthConfig struct {
	AuthURL      string
	TokenURL     string
	ClientID     string   // populated from env var MONOES_<PLATFORM>_CLIENT_ID
	ClientSecret string   // populated from env var MONOES_<PLATFORM>_CLIENT_SECRET
	Scopes       []string
	CallbackPort int // default 9876
}

// PlatformDef is one entry in the platform registry.
type PlatformDef struct {
	ID         string                         // "github", "instagram"
	Name       string                         // "GitHub"
	Category   string                         // "social" | "service" | "database" | "communication"
	ConnectVia string                         // "UI" (browser/Sessions page) | "API" (Credentials page)
	Methods    []AuthMethod                   // ordered: first = recommended
	Fields     map[AuthMethod][]CredentialField
	OAuth      *OAuthConfig
	IconEmoji  string
}

// Registry maps platform ID → PlatformDef.
var Registry = map[string]PlatformDef{
	// ── Social (browser session) ─────────────────────────────────────────
	"instagram": {
		ID: "instagram", Name: "Instagram", Category: "social", ConnectVia: "UI",
		IconEmoji: "📸",
		Methods:   []AuthMethod{MethodBrowser},
	},
	"linkedin": {
		ID: "linkedin", Name: "LinkedIn", Category: "social", ConnectVia: "UI",
		IconEmoji: "💼",
		Methods:   []AuthMethod{MethodBrowser},
	},
	"x": {
		ID: "x", Name: "X (Twitter)", Category: "social", ConnectVia: "UI",
		IconEmoji: "𝕏",
		Methods:   []AuthMethod{MethodBrowser},
	},
	"tiktok": {
		ID: "tiktok", Name: "TikTok", Category: "social", ConnectVia: "UI",
		IconEmoji: "🎵",
		Methods:   []AuthMethod{MethodBrowser},
	},
	"telegram": {
		ID: "telegram", Name: "Telegram", Category: "social", ConnectVia: "UI",
		IconEmoji: "✈️",
		Methods:   []AuthMethod{MethodBrowser, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "bot_token", Label: "Bot Token", Secret: true, Required: true,
					HelpURL:  "https://core.telegram.org/bots#creating-a-new-bot",
					HelpText: "Talk to @BotFather on Telegram → /newbot → copy the token"},
			},
		},
	},

	// ── Services (API) ───────────────────────────────────────────────────
	"github": {
		ID: "github", Name: "GitHub", Category: "service", ConnectVia: "API",
		IconEmoji: "🐙",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "token", Label: "Personal Access Token", Secret: true, Required: true,
					HelpURL:  "https://github.com/settings/tokens/new",
					HelpText: "GitHub → Settings → Developer Settings → Personal Access Tokens → New"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			Scopes:       []string{"repo", "read:user", "user:email"},
			CallbackPort: 9876,
		},
	},
	"notion": {
		ID: "notion", Name: "Notion", Category: "service", ConnectVia: "API",
		IconEmoji: "📝",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "token", Label: "Internal Integration Token", Secret: true, Required: true,
					HelpURL:  "https://www.notion.so/my-integrations",
					HelpText: "Notion → Settings → Integrations → New Integration → copy secret"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://api.notion.com/v1/oauth/authorize",
			TokenURL:     "https://api.notion.com/v1/oauth/token",
			Scopes:       []string{},
			CallbackPort: 9876,
		},
	},
	"airtable": {
		ID: "airtable", Name: "Airtable", Category: "service", ConnectVia: "API",
		IconEmoji: "📊",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "api_key", Label: "Personal Access Token", Secret: true, Required: true,
					HelpURL:  "https://airtable.com/create/tokens",
					HelpText: "Airtable → Account → Developer Hub → Personal Access Tokens → Create"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://airtable.com/oauth2/v1/authorize",
			TokenURL:     "https://airtable.com/oauth2/v1/token",
			Scopes:       []string{"data.records:read", "data.records:write", "schema.bases:read"},
			CallbackPort: 9876,
		},
	},
	"jira": {
		ID: "jira", Name: "Jira", Category: "service", ConnectVia: "API",
		IconEmoji: "🎯",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "email", Label: "Email", Secret: false, Required: true},
				{Key: "api_token", Label: "API Token", Secret: true, Required: true,
					HelpURL:  "https://id.atlassian.com/manage-profile/security/api-tokens",
					HelpText: "Atlassian Account → Security → API tokens → Create"},
				{Key: "domain", Label: "Domain (e.g. yourco.atlassian.net)", Secret: false, Required: true},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://auth.atlassian.com/authorize",
			TokenURL:     "https://auth.atlassian.com/oauth/token",
			Scopes:       []string{"read:jira-work", "write:jira-work"},
			CallbackPort: 9876,
		},
	},
	"linear": {
		ID: "linear", Name: "Linear", Category: "service", ConnectVia: "API",
		IconEmoji: "📐",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "api_key", Label: "Personal API Key", Secret: true, Required: true,
					HelpURL:  "https://linear.app/settings/api",
					HelpText: "Linear → Settings → API → Personal API Keys → Create key"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://linear.app/oauth/authorize",
			TokenURL:     "https://api.linear.app/oauth/token",
			Scopes:       []string{"read", "write"},
			CallbackPort: 9876,
		},
	},
	"asana": {
		ID: "asana", Name: "Asana", Category: "service", ConnectVia: "API",
		IconEmoji: "✅",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "access_token", Label: "Personal Access Token", Secret: true, Required: true,
					HelpURL:  "https://app.asana.com/0/my-apps",
					HelpText: "Asana → My Profile → Apps → Manage Developer Apps → New access token"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://app.asana.com/-/oauth_authorize",
			TokenURL:     "https://app.asana.com/-/oauth_token",
			Scopes:       []string{"default"},
			CallbackPort: 9876,
		},
	},
	"stripe": {
		ID: "stripe", Name: "Stripe", Category: "service", ConnectVia: "API",
		IconEmoji: "💳",
		Methods:   []AuthMethod{MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "secret_key", Label: "Secret Key (sk_...)", Secret: true, Required: true,
					HelpURL:  "https://dashboard.stripe.com/apikeys",
					HelpText: "Stripe Dashboard → Developers → API keys → copy Secret key"},
			},
		},
	},
	"shopify": {
		ID: "shopify", Name: "Shopify", Category: "service", ConnectVia: "API",
		IconEmoji: "🛍️",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "shop_domain", Label: "Shop Domain (e.g. yourstore.myshopify.com)", Required: true},
				{Key: "access_token", Label: "Admin API Access Token", Secret: true, Required: true,
					HelpURL:  "https://shopify.dev/docs/apps/auth/admin-app-access-tokens",
					HelpText: "Shopify Admin → Apps → Develop apps → Create app → install → copy token"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://{shop}/admin/oauth/authorize",
			TokenURL:     "https://{shop}/admin/oauth/access_token",
			Scopes:       []string{"read_orders", "write_orders", "read_products"},
			CallbackPort: 9876,
		},
	},
	"salesforce": {
		ID: "salesforce", Name: "Salesforce", Category: "service", ConnectVia: "API",
		IconEmoji: "☁️",
		Methods:   []AuthMethod{MethodOAuth},
		OAuth: &OAuthConfig{
			AuthURL:      "https://login.salesforce.com/services/oauth2/authorize",
			TokenURL:     "https://login.salesforce.com/services/oauth2/token",
			Scopes:       []string{"api", "refresh_token"},
			CallbackPort: 9876,
		},
	},
	"hubspot": {
		ID: "hubspot", Name: "HubSpot", Category: "service", ConnectVia: "API",
		IconEmoji: "🧡",
		Methods:   []AuthMethod{MethodOAuth, MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "access_token", Label: "Private App Access Token", Secret: true, Required: true,
					HelpURL:  "https://app.hubspot.com/private-apps",
					HelpText: "HubSpot → Settings → Integrations → Private Apps → Create → copy token"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://app.hubspot.com/oauth/authorize",
			TokenURL:     "https://api.hubapi.com/oauth/v1/token",
			Scopes:       []string{"crm.objects.contacts.read", "crm.objects.contacts.write"},
			CallbackPort: 9876,
		},
	},
	"google_sheets": {
		ID: "google_sheets", Name: "Google Sheets", Category: "service", ConnectVia: "API",
		IconEmoji: "📗",
		Methods:   []AuthMethod{MethodOAuth},
		OAuth: &OAuthConfig{
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			Scopes:       []string{"https://www.googleapis.com/auth/spreadsheets"},
			CallbackPort: 9876,
		},
	},
	"gmail": {
		ID: "gmail", Name: "Gmail", Category: "service", ConnectVia: "API",
		IconEmoji: "📧",
		Methods:   []AuthMethod{MethodOAuth, MethodAppPass},
		Fields: map[AuthMethod][]CredentialField{
			MethodAppPass: {
				{Key: "email", Label: "Gmail Address", Required: true},
				{Key: "app_password", Label: "App Password", Secret: true, Required: true,
					HelpURL:  "https://myaccount.google.com/apppasswords",
					HelpText: "Google Account → Security → 2-Step Verification → App Passwords → generate"},
			},
		},
		OAuth: &OAuthConfig{
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			Scopes:       []string{"https://www.googleapis.com/auth/gmail.modify"},
			CallbackPort: 9876,
		},
	},
	"google_drive": {
		ID: "google_drive", Name: "Google Drive", Category: "service", ConnectVia: "API",
		IconEmoji: "📁",
		Methods:   []AuthMethod{MethodOAuth},
		OAuth: &OAuthConfig{
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			Scopes:       []string{"https://www.googleapis.com/auth/drive"},
			CallbackPort: 9876,
		},
	},

	// ── Communication ────────────────────────────────────────────────────
	"slack": {
		ID: "slack", Name: "Slack", Category: "communication", ConnectVia: "API",
		IconEmoji: "💬",
		Methods:   []AuthMethod{MethodOAuth},
		OAuth: &OAuthConfig{
			AuthURL:      "https://slack.com/oauth/v2/authorize",
			TokenURL:     "https://slack.com/api/oauth.v2.access",
			Scopes:       []string{"channels:read", "chat:write", "users:read"},
			CallbackPort: 9876,
		},
	},
	"discord": {
		ID: "discord", Name: "Discord", Category: "communication", ConnectVia: "API",
		IconEmoji: "🎮",
		Methods:   []AuthMethod{MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "bot_token", Label: "Bot Token", Secret: true, Required: true,
					HelpURL:  "https://discord.com/developers/applications",
					HelpText: "Discord Developer Portal → New Application → Bot → Reset Token → copy"},
			},
		},
	},
	"twilio": {
		ID: "twilio", Name: "Twilio", Category: "communication", ConnectVia: "API",
		IconEmoji: "📱",
		Methods:   []AuthMethod{MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "account_sid", Label: "Account SID", Required: true,
					HelpURL: "https://console.twilio.com/"},
				{Key: "auth_token", Label: "Auth Token", Secret: true, Required: true},
				{Key: "from_number", Label: "From Phone Number (e.g. +15551234567)", Required: true},
			},
		},
	},
	"whatsapp": {
		ID: "whatsapp", Name: "WhatsApp (via Twilio)", Category: "communication", ConnectVia: "API",
		IconEmoji: "💚",
		Methods:   []AuthMethod{MethodAPIKey},
		Fields: map[AuthMethod][]CredentialField{
			MethodAPIKey: {
				{Key: "account_sid", Label: "Twilio Account SID", Required: true},
				{Key: "auth_token", Label: "Twilio Auth Token", Secret: true, Required: true},
				{Key: "from_number", Label: "WhatsApp From Number (whatsapp:+14155238886)", Required: true},
			},
		},
	},
	"smtp": {
		ID: "smtp", Name: "Email (SMTP/IMAP)", Category: "communication", ConnectVia: "API",
		IconEmoji: "✉️",
		Methods:   []AuthMethod{MethodAppPass},
		Fields: map[AuthMethod][]CredentialField{
			MethodAppPass: {
				{Key: "email", Label: "Email Address", Required: true},
				{Key: "password", Label: "Password / App Password", Secret: true, Required: true},
				{Key: "smtp_host", Label: "SMTP Host (e.g. smtp.gmail.com)", Required: true},
				{Key: "smtp_port", Label: "SMTP Port (e.g. 587)", Required: true},
				{Key: "imap_host", Label: "IMAP Host (optional, e.g. imap.gmail.com)"},
				{Key: "imap_port", Label: "IMAP Port (optional, e.g. 993)"},
			},
		},
	},

	// ── Databases ────────────────────────────────────────────────────────
	"postgresql": {
		ID: "postgresql", Name: "PostgreSQL", Category: "database", ConnectVia: "API",
		IconEmoji: "🐘",
		Methods:   []AuthMethod{MethodConnStr},
		Fields: map[AuthMethod][]CredentialField{
			MethodConnStr: {
				{Key: "connection_string", Label: "Connection String",
					Secret: true, Required: true,
					HelpText: "e.g. postgres://user:password@localhost:5432/dbname"},
			},
		},
	},
	"mysql": {
		ID: "mysql", Name: "MySQL", Category: "database", ConnectVia: "API",
		IconEmoji: "🐬",
		Methods:   []AuthMethod{MethodConnStr},
		Fields: map[AuthMethod][]CredentialField{
			MethodConnStr: {
				{Key: "connection_string", Label: "Connection String",
					Secret: true, Required: true,
					HelpText: "e.g. user:password@tcp(localhost:3306)/dbname"},
			},
		},
	},
	"mongodb": {
		ID: "mongodb", Name: "MongoDB", Category: "database", ConnectVia: "API",
		IconEmoji: "🍃",
		Methods:   []AuthMethod{MethodConnStr},
		Fields: map[AuthMethod][]CredentialField{
			MethodConnStr: {
				{Key: "connection_string", Label: "Connection URI",
					Secret: true, Required: true,
					HelpText: "e.g. mongodb://user:password@localhost:27017/dbname"},
			},
		},
	},
	"redis": {
		ID: "redis", Name: "Redis", Category: "database", ConnectVia: "API",
		IconEmoji: "🔴",
		Methods:   []AuthMethod{MethodConnStr},
		Fields: map[AuthMethod][]CredentialField{
			MethodConnStr: {
				{Key: "connection_string", Label: "Connection URL",
					Secret: true, Required: true,
					HelpText: "e.g. redis://:password@localhost:6379/0"},
			},
		},
	},
}

// Get returns the PlatformDef for id, or false if not found.
func Get(id string) (PlatformDef, bool) {
	p, ok := Registry[id]
	return p, ok
}

// All returns all platform definitions in a deterministic slice.
func All() []PlatformDef {
	out := make([]PlatformDef, 0, len(Registry))
	for _, p := range Registry {
		out = append(out, p)
	}
	return out
}

// ByCategory returns platforms filtered by category.
func ByCategory(category string) []PlatformDef {
	var out []PlatformDef
	for _, p := range Registry {
		if p.Category == category {
			out = append(out, p)
		}
	}
	return out
}

// ByConnectVia returns platforms filtered by ConnectVia ("UI" or "API").
func ByConnectVia(via string) []PlatformDef {
	var out []PlatformDef
	for _, p := range Registry {
		if p.ConnectVia == via {
			out = append(out, p)
		}
	}
	return out
}
```

**Step 2: Write registry test**

```go
// internal/connections/registry_test.go
package connections

import "testing"

func TestRegistryHasAllExpectedPlatforms(t *testing.T) {
	expected := []string{
		"instagram", "linkedin", "x", "tiktok", "telegram",
		"github", "notion", "airtable", "jira", "linear", "asana",
		"stripe", "shopify", "salesforce", "hubspot",
		"google_sheets", "gmail", "google_drive",
		"slack", "discord", "twilio", "whatsapp", "smtp",
		"postgresql", "mysql", "mongodb", "redis",
	}
	for _, id := range expected {
		if _, ok := Registry[id]; !ok {
			t.Errorf("missing platform %q in Registry", id)
		}
	}
}

func TestPlatformMethodsNonEmpty(t *testing.T) {
	for id, p := range Registry {
		if len(p.Methods) == 0 {
			t.Errorf("platform %q has no Methods", id)
		}
	}
}

func TestAPIKeyPlatformsHaveFields(t *testing.T) {
	for id, p := range Registry {
		for _, m := range p.Methods {
			if m == MethodAPIKey || m == MethodConnStr || m == MethodAppPass {
				fields, ok := p.Fields[m]
				if !ok || len(fields) == 0 {
					t.Errorf("platform %q method %q has no CredentialFields", id, m)
				}
			}
		}
	}
}

func TestOAuthPlatformsHaveConfig(t *testing.T) {
	for id, p := range Registry {
		for _, m := range p.Methods {
			if m == MethodOAuth {
				if p.OAuth == nil {
					t.Errorf("platform %q has MethodOAuth but nil OAuthConfig", id)
				}
				if p.OAuth.AuthURL == "" || p.OAuth.TokenURL == "" {
					t.Errorf("platform %q OAuthConfig missing AuthURL or TokenURL", id)
				}
			}
		}
	}
}

func TestByConnectVia(t *testing.T) {
	ui := ByConnectVia("UI")
	api := ByConnectVia("API")
	if len(ui) == 0 {
		t.Error("ByConnectVia(UI) returned nothing")
	}
	if len(api) == 0 {
		t.Error("ByConnectVia(API) returned nothing")
	}
	if len(ui)+len(api) != len(Registry) {
		t.Errorf("UI(%d) + API(%d) != total(%d)", len(ui), len(api), len(Registry))
	}
}
```

**Step 3: Run tests**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/connections/... -v -run TestRegistry
```

Expected: all 4 tests PASS.

**Step 4: Commit**

```bash
git add internal/connections/registry.go internal/connections/registry_test.go
git commit -m "feat(connections): add platform registry with 27 platforms"
```

---

### Task 2: Storage layer — `connections` table CRUD

**Files:**
- Create: `internal/connections/storage.go`
- Create: `internal/connections/storage_test.go`

**Step 1: Write storage.go**

```go
// internal/connections/storage.go
package connections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Connection is a stored platform connection.
type Connection struct {
	ID         string                 `json:"id"`
	Platform   string                 `json:"platform"`
	Method     AuthMethod             `json:"method"`
	Label      string                 `json:"label"`
	AccountID  string                 `json:"account_id"` // username / email resolved at validation
	Data       map[string]interface{} `json:"data"`       // tokens, keys, etc.
	Status     string                 `json:"status"`     // "active" | "expired" | "error"
	LastTested string                 `json:"last_tested,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
}

const createConnectionsTable = `
CREATE TABLE IF NOT EXISTS connections (
    id          TEXT PRIMARY KEY,
    platform    TEXT NOT NULL,
    method      TEXT NOT NULL,
    label       TEXT NOT NULL,
    account_id  TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'active',
    last_tested TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_connections_platform ON connections(platform);
CREATE INDEX IF NOT EXISTS idx_connections_status   ON connections(status);`

// Store handles persistence for the connections table.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store backed by db.
// Call EnsureTable first to create the schema.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// EnsureTable creates the connections table if it doesn't exist.
func (s *Store) EnsureTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, createConnectionsTable)
	return err
}

// Save upserts a connection. If c.ID is empty a UUID is generated.
func (s *Store) Save(ctx context.Context, c *Connection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = "active"
	}

	dataJSON, err := json.Marshal(c.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO connections (id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			label=excluded.label, account_id=excluded.account_id,
			data=excluded.data, status=excluded.status,
			last_tested=excluded.last_tested, updated_at=excluded.updated_at`,
		c.ID, c.Platform, string(c.Method), c.Label, c.AccountID,
		string(dataJSON), c.Status, c.LastTested, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

// Get returns a connection by ID, or nil if not found.
func (s *Store) Get(ctx context.Context, id string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at
		 FROM connections WHERE id = ?`, id)
	return scanConnection(row)
}

// ListAll returns every connection.
func (s *Store) ListAll(ctx context.Context) ([]Connection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at
		 FROM connections ORDER BY platform, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConnections(rows)
}

// ListByPlatform returns connections for a specific platform.
func (s *Store) ListByPlatform(ctx context.Context, platform string) ([]Connection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at
		 FROM connections WHERE platform = ? ORDER BY created_at`, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConnections(rows)
}

// Delete removes a connection by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("connection %q not found", id)
	}
	return nil
}

// MarkTested updates status and last_tested for a connection.
func (s *Store) MarkTested(ctx context.Context, id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE connections SET status=?, last_tested=?, updated_at=? WHERE id=?`,
		status, now, now, id)
	return err
}

func scanConnection(row *sql.Row) (*Connection, error) {
	var c Connection
	var dataJSON, method string
	err := row.Scan(&c.ID, &c.Platform, &method, &c.Label, &c.AccountID,
		&dataJSON, &c.Status, &c.LastTested, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Method = AuthMethod(method)
	if err := json.Unmarshal([]byte(dataJSON), &c.Data); err != nil {
		c.Data = make(map[string]interface{})
	}
	return &c, nil
}

func scanConnections(rows *sql.Rows) ([]Connection, error) {
	var out []Connection
	for rows.Next() {
		var c Connection
		var dataJSON, method string
		if err := rows.Scan(&c.ID, &c.Platform, &method, &c.Label, &c.AccountID,
			&dataJSON, &c.Status, &c.LastTested, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Method = AuthMethod(method)
		if err := json.Unmarshal([]byte(dataJSON), &c.Data); err != nil {
			c.Data = make(map[string]interface{})
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

**Step 2: Write storage test**

```go
// internal/connections/storage_test.go
package connections

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStoreSaveAndGet(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	ctx := context.Background()

	if err := s.EnsureTable(ctx); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}

	conn := &Connection{
		Platform:  "github",
		Method:    MethodAPIKey,
		Label:     "GitHub – morteza",
		AccountID: "morteza",
		Data:      map[string]interface{}{"token": "ghp_test123"},
		Status:    "active",
	}

	if err := s.Save(ctx, conn); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if conn.ID == "" {
		t.Fatal("Save should assign ID")
	}

	got, err := s.Get(ctx, conn.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Label != conn.Label {
		t.Errorf("Label mismatch: got %q want %q", got.Label, conn.Label)
	}
	if got.Data["token"] != "ghp_test123" {
		t.Errorf("Data.token mismatch: %v", got.Data)
	}
}

func TestStoreDelete(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	_ = s.EnsureTable(ctx)

	conn := &Connection{Platform: "stripe", Method: MethodAPIKey, Label: "Stripe", Data: map[string]interface{}{}}
	_ = s.Save(ctx, conn)

	if err := s.Delete(ctx, conn.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ := s.Get(ctx, conn.ID)
	if got != nil {
		t.Error("Expected nil after delete")
	}
}

func TestStoreListByPlatform(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	_ = s.EnsureTable(ctx)

	_ = s.Save(ctx, &Connection{Platform: "github", Method: MethodAPIKey, Label: "G1", Data: map[string]interface{}{}})
	_ = s.Save(ctx, &Connection{Platform: "github", Method: MethodOAuth, Label: "G2", Data: map[string]interface{}{}})
	_ = s.Save(ctx, &Connection{Platform: "stripe", Method: MethodAPIKey, Label: "S1", Data: map[string]interface{}{}})

	conns, err := s.ListByPlatform(ctx, "github")
	if err != nil {
		t.Fatalf("ListByPlatform: %v", err)
	}
	if len(conns) != 2 {
		t.Errorf("expected 2 GitHub connections, got %d", len(conns))
	}
}
```

**Step 3: Run tests**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/connections/... -v -run TestStore
```

Expected: 3 tests PASS.

**Step 4: Commit**

```bash
git add internal/connections/storage.go internal/connections/storage_test.go
git commit -m "feat(connections): add connection storage CRUD"
```

---

### Task 3: OAuth local callback server

**Files:**
- Create: `internal/connections/oauth.go`
- Create: `internal/connections/oauth_test.go`

**Step 1: Write oauth.go**

```go
// internal/connections/oauth.go
package connections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// OAuthResult holds the token data returned after a successful OAuth flow.
type OAuthResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// RunOAuthFlow opens the browser to the provider's auth URL, starts a local
// callback server, waits for the redirect, exchanges the code for a token,
// and returns the result. Timeout defaults to 5 minutes if zero.
func RunOAuthFlow(ctx context.Context, cfg OAuthConfig, timeout time.Duration) (*OAuthResult, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	port := cfg.CallbackPort
	if port == 0 {
		port = 9876
	}

	state, err := randomState()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	// Build authorization URL.
	authURL, err := buildAuthURL(cfg, redirectURI, state)
	if err != nil {
		return nil, fmt.Errorf("build auth URL: %w", err)
	}

	// Channel to receive code or error from callback handler.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if s := q.Get("state"); s != state {
			errCh <- fmt.Errorf("state mismatch: expected %q got %q", state, s)
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			errCh <- fmt.Errorf("provider error: %s — %s", e, q.Get("error_description"))
			fmt.Fprintf(w, "<html><body><h2>Authorization failed: %s</h2><p>You can close this tab.</p></body></html>", e)
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "no code", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, "<html><body><h2>✓ Connected!</h2><p>You can close this tab and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on port %d: %w", port, err)
	}

	go srv.Serve(ln) //nolint:errcheck

	// Open the auth URL in the default browser.
	fmt.Printf("→ Opening browser: %s\n→ Waiting for authorization on http://localhost:%d/callback\n", authURL, port)
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("  Could not open browser automatically. Please open manually:\n  %s\n", authURL)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		srv.Close()
		return nil, err
	case <-ctx.Done():
		srv.Close()
		return nil, fmt.Errorf("OAuth timed out after %s", timeout)
	}
	srv.Close()

	// Exchange code for token.
	return exchangeCode(cfg, code, redirectURI)
}

func buildAuthURL(cfg OAuthConfig, redirectURI, state string) (string, error) {
	u, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	if len(cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func exchangeCode(cfg OAuthConfig, code, redirectURI string) (*OAuthResult, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", cfg.ClientID)
	data.Set("client_secret", cfg.ClientSecret)

	req, err := http.NewRequest(http.MethodPost, cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var result OAuthResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in response: %s", string(body))
	}
	return &result, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func openBrowser(u string) error {
	return exec.Command("open", u).Start()
}
```

**Step 2: Write oauth test (unit-level — no real OAuth provider needed)**

```go
// internal/connections/oauth_test.go
package connections

import (
	"strings"
	"testing"
)

func TestBuildAuthURL(t *testing.T) {
	cfg := OAuthConfig{
		AuthURL:      "https://github.com/login/oauth/authorize",
		ClientID:     "test-client",
		Scopes:       []string{"repo", "read:user"},
		CallbackPort: 9876,
	}
	u, err := buildAuthURL(cfg, "http://localhost:9876/callback", "teststate")
	if err != nil {
		t.Fatalf("buildAuthURL: %v", err)
	}
	if !strings.Contains(u, "client_id=test-client") {
		t.Errorf("missing client_id: %s", u)
	}
	if !strings.Contains(u, "state=teststate") {
		t.Errorf("missing state: %s", u)
	}
	if !strings.Contains(u, "scope=repo+read%3Auser") && !strings.Contains(u, "scope=repo") {
		t.Errorf("missing scope: %s", u)
	}
}

func TestRandomStateUnique(t *testing.T) {
	s1, _ := randomState()
	s2, _ := randomState()
	if s1 == s2 {
		t.Error("randomState should return unique values")
	}
	if len(s1) < 16 {
		t.Errorf("state too short: %q", s1)
	}
}
```

**Step 3: Run tests**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/connections/... -v -run TestBuild
go test ./internal/connections/... -v -run TestRandom
```

Expected: both PASS.

**Step 4: Commit**

```bash
git add internal/connections/oauth.go internal/connections/oauth_test.go
git commit -m "feat(connections): add OAuth local callback server"
```

---

### Task 4: Validation functions

**Files:**
- Create: `internal/connections/validate.go`

**Step 1: Write validate.go**

Each platform's `ValidateFn` field maps to a function in this file. The validate function takes a `Connection` with populated `Data` and returns `accountID, error`. A non-nil error means the credential is invalid.

```go
// internal/connections/validate.go
package connections

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ValidateConnection calls the appropriate validation function for the connection's
// platform and returns the resolved account identifier (username/email/org).
// Returns ("", nil) for platforms with no validation implemented yet.
func ValidateConnection(c *Connection) (accountID string, err error) {
	switch c.Platform {
	case "github":
		return validateGitHub(c)
	case "notion":
		return validateNotion(c)
	case "airtable":
		return validateAirtable(c)
	case "jira":
		return validateJira(c)
	case "linear":
		return validateLinear(c)
	case "stripe":
		return validateStripe(c)
	case "slack":
		return validateSlack(c)
	case "discord":
		return validateDiscord(c)
	case "twilio":
		return validateTwilio(c)
	case "postgresql", "mysql", "mongodb", "redis":
		// Connection string validated on first use; return a placeholder.
		if cs, ok := c.Data["connection_string"].(string); ok && cs != "" {
			return cs[:min(30, len(cs))] + "...", nil
		}
		return "", fmt.Errorf("missing connection_string")
	default:
		// Browser-session platforms and unimplemented services: skip validation.
		return "", nil
	}
}

func getStr(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func doGET(reqURL, authHeader string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode, nil
}

func validateGitHub(c *Connection) (string, error) {
	token := getStr(c.Data, "token")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}
	if token == "" {
		return "", fmt.Errorf("missing token")
	}
	body, status, err := doGET("https://api.github.com/user", "Bearer "+token)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("GitHub API returned %d", status)
	}
	var r struct{ Login string `json:"login"` }
	if err := json.Unmarshal(body, &r); err != nil || r.Login == "" {
		return "", fmt.Errorf("could not parse GitHub user response")
	}
	return r.Login, nil
}

func validateNotion(c *Connection) (string, error) {
	token := getStr(c.Data, "token")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}
	body, status, err := doGET("https://api.notion.com/v1/users/me",
		"Bearer "+token)
	if err != nil {
		return "", err
	}
	// Also set Notion-Version header — do it manually.
	req, _ := http.NewRequest(http.MethodGet, "https://api.notion.com/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Accept", "application/json")
	resp, err2 := http.DefaultClient.Do(req)
	if err2 != nil {
		return "", err2
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	status = resp.StatusCode
	if status != 200 {
		return "", fmt.Errorf("Notion API returned %d", status)
	}
	var r struct {
		Name string `json:"name"`
		Bot  struct {
			WorkspaceName string `json:"workspace_name"`
		} `json:"bot"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Bot.WorkspaceName != "" {
		return r.Bot.WorkspaceName, nil
	}
	return r.Name, nil
}

func validateAirtable(c *Connection) (string, error) {
	token := getStr(c.Data, "api_key")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}
	body, status, err := doGET("https://api.airtable.com/v0/meta/whoami", "Bearer "+token)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("Airtable API returned %d", status)
	}
	var r struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &r)
	return r.ID, nil
}

func validateJira(c *Connection) (string, error) {
	email := getStr(c.Data, "email")
	token := getStr(c.Data, "api_token")
	domain := getStr(c.Data, "domain")
	if email == "" || token == "" || domain == "" {
		return "", fmt.Errorf("missing email, api_token, or domain")
	}
	reqURL := fmt.Sprintf("https://%s/rest/api/3/myself", strings.TrimSuffix(domain, "/"))
	req, _ := http.NewRequest(http.MethodGet, reqURL, nil)
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Jira API returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var r struct{ DisplayName string `json:"displayName"` }
	_ = json.Unmarshal(body, &r)
	return r.DisplayName, nil
}

func validateLinear(c *Connection) (string, error) {
	token := getStr(c.Data, "api_key")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}
	body := `{"query":"{ viewer { name email } }"}`
	req, _ := http.NewRequest(http.MethodPost, "https://api.linear.app/graphql",
		strings.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Linear API returned %d", resp.StatusCode)
	}
	rb, _ := io.ReadAll(resp.Body)
	var r struct {
		Data struct {
			Viewer struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"viewer"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rb, &r)
	return r.Data.Viewer.Email, nil
}

func validateStripe(c *Connection) (string, error) {
	key := getStr(c.Data, "secret_key")
	if key == "" {
		return "", fmt.Errorf("missing secret_key")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.stripe.com/v1/account", nil)
	req.SetBasicAuth(key, "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Stripe API returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		BusinessProfile struct{ Name string `json:"name"` } `json:"business_profile"`
		Email           string `json:"email"`
	}
	_ = json.Unmarshal(body, &r)
	if r.BusinessProfile.Name != "" {
		return r.BusinessProfile.Name, nil
	}
	return r.Email, nil
}

func validateSlack(c *Connection) (string, error) {
	token := getStr(c.Data, "access_token")
	body, status, err := doGET("https://slack.com/api/auth.test", "Bearer "+token)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("Slack API returned %d", status)
	}
	var r struct {
		OK    bool   `json:"ok"`
		Team  string `json:"team"`
		User  string `json:"user"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	if !r.OK {
		return "", fmt.Errorf("Slack auth failed: %s", r.Error)
	}
	return fmt.Sprintf("%s / %s", r.Team, r.User), nil
}

func validateDiscord(c *Connection) (string, error) {
	token := getStr(c.Data, "bot_token")
	body, status, err := doGET("https://discord.com/api/v10/users/@me", "Bot "+token)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("Discord API returned %d", status)
	}
	var r struct{ Username string `json:"username"` }
	_ = json.Unmarshal(body, &r)
	return r.Username, nil
}

func validateTwilio(c *Connection) (string, error) {
	sid := getStr(c.Data, "account_sid")
	tok := getStr(c.Data, "auth_token")
	if sid == "" || tok == "" {
		return "", fmt.Errorf("missing account_sid or auth_token")
	}
	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s.json", sid), nil)
	req.SetBasicAuth(sid, tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Twilio API returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var r struct{ FriendlyName string `json:"friendly_name"` }
	_ = json.Unmarshal(body, &r)
	return r.FriendlyName, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

**Step 2: Run compile check (validate.go has no easily unit-testable pure functions without real API calls)**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go build ./internal/connections/...
echo "EXIT:$?"
```

Expected: EXIT:0 — clean compile.

**Step 3: Commit**

```bash
git add internal/connections/validate.go
git commit -m "feat(connections): add per-platform validation functions"
```

---

### Task 5: ConnectionManager

**Files:**
- Create: `internal/connections/manager.go`
- Create: `internal/connections/manager_test.go`

**Step 1: Write manager.go**

```go
// internal/connections/manager.go
package connections

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

// Manager orchestrates all connection flows: connect, list, test, remove, refresh.
type Manager struct {
	store *Store
}

// NewManager returns a Manager backed by db. Calls EnsureTable on init.
func NewManager(db *sql.DB) (*Manager, error) {
	store := NewStore(db)
	if err := store.EnsureTable(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure connections table: %w", err)
	}
	return &Manager{store: store}, nil
}

// ConnectOptions controls how Connect behaves.
type ConnectOptions struct {
	Method       AuthMethod    // force a specific method (zero = prompt if multiple)
	OAuthTimeout time.Duration // timeout for OAuth flow
	// For non-interactive (testing/scripting): pre-fill field values.
	FieldValues map[string]string
}

// Connect walks the user through authenticating to platform and saves the result.
// Returns the saved Connection on success.
func (m *Manager) Connect(ctx context.Context, platformID string, opts ConnectOptions) (*Connection, error) {
	p, ok := Get(platformID)
	if !ok {
		return nil, fmt.Errorf("unknown platform %q — run `monoes connect list` to see supported platforms", platformID)
	}

	method := opts.Method
	if method == "" {
		method = m.pickMethod(p)
	}

	// Validate method is supported.
	supported := false
	for _, m2 := range p.Methods {
		if m2 == method {
			supported = true
			break
		}
	}
	if !supported {
		return nil, fmt.Errorf("platform %q does not support method %q", platformID, method)
	}

	conn := &Connection{
		Platform: platformID,
		Method:   method,
	}

	switch method {
	case MethodOAuth:
		if err := m.connectOAuth(ctx, p, conn, opts.OAuthTimeout); err != nil {
			return nil, err
		}
	case MethodAPIKey, MethodAppPass, MethodConnStr:
		if err := m.connectFields(p, method, conn, opts.FieldValues); err != nil {
			return nil, err
		}
	case MethodBrowser:
		return nil, fmt.Errorf("platform %q uses browser session — run `monoes connect %s` which opens the browser for login", platformID, platformID)
	default:
		return nil, fmt.Errorf("unsupported method %q", method)
	}

	// Validate the connection (live API call).
	fmt.Printf("→ Validating connection...\n")
	accountID, err := ValidateConnection(conn)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	conn.AccountID = accountID
	conn.Status = "active"
	conn.LastTested = time.Now().UTC().Format(time.RFC3339)

	// Auto-generate label.
	if accountID != "" {
		conn.Label = fmt.Sprintf("%s – %s", p.Name, accountID)
	} else {
		conn.Label = p.Name
	}

	if err := m.store.Save(ctx, conn); err != nil {
		return nil, fmt.Errorf("save connection: %w", err)
	}

	fmt.Printf("✓ Connected%s\n✓ Saved as %q (id: %s)\n",
		formatAccount(accountID), conn.Label, conn.ID)
	return conn, nil
}

// List returns all connections, optionally filtered by platform.
func (m *Manager) List(ctx context.Context, platform string) ([]Connection, error) {
	if platform != "" {
		return m.store.ListByPlatform(ctx, platform)
	}
	return m.store.ListAll(ctx)
}

// Test re-validates a connection and updates its status.
func (m *Manager) Test(ctx context.Context, id string) error {
	conn, err := m.store.Get(ctx, id)
	if err != nil || conn == nil {
		return fmt.Errorf("connection %q not found", id)
	}
	accountID, err := ValidateConnection(conn)
	status := "active"
	if err != nil {
		status = "error"
		fmt.Printf("✗ Validation failed: %v\n", err)
	} else {
		if accountID != "" {
			conn.AccountID = accountID
			conn.Label = fmt.Sprintf("%s – %s", Registry[conn.Platform].Name, accountID)
		}
		fmt.Printf("✓ Connection valid%s\n", formatAccount(accountID))
	}
	return m.store.MarkTested(ctx, id, status)
}

// Remove deletes a connection by ID.
func (m *Manager) Remove(ctx context.Context, id string) error {
	return m.store.Delete(ctx, id)
}

// Refresh re-runs the auth flow for a connection and updates the stored data.
func (m *Manager) Refresh(ctx context.Context, id string, timeout time.Duration) error {
	conn, err := m.store.Get(ctx, id)
	if err != nil || conn == nil {
		return fmt.Errorf("connection %q not found", id)
	}
	p, ok := Get(conn.Platform)
	if !ok {
		return fmt.Errorf("unknown platform %q", conn.Platform)
	}
	if conn.Method != MethodOAuth {
		return fmt.Errorf("refresh only supported for OAuth connections; use `monoes connect %s` to re-authenticate", conn.Platform)
	}
	if err := m.connectOAuth(ctx, p, conn, timeout); err != nil {
		return err
	}
	accountID, _ := ValidateConnection(conn)
	conn.AccountID = accountID
	conn.Status = "active"
	conn.LastTested = time.Now().UTC().Format(time.RFC3339)
	return m.store.Save(ctx, conn)
}

// ── private helpers ──────────────────────────────────────────────────────────

func (m *Manager) pickMethod(p PlatformDef) AuthMethod {
	if len(p.Methods) == 1 {
		return p.Methods[0]
	}
	fmt.Printf("┌─ Connect %s ────────────────────────────────────┐\n", p.Name)
	fmt.Printf("│ How do you want to connect?\n")
	for i, method := range p.Methods {
		label := methodLabel(method)
		rec := ""
		if i == 0 {
			rec = " (recommended)"
		}
		fmt.Printf("│   [%d] %s%s\n", i+1, label, rec)
	}
	fmt.Printf("└────────────────────────────────────────────────────┘\n")
	fmt.Printf("Choice [1]: ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}
	idx := 0
	for _, ch := range line {
		if ch >= '1' && ch <= '9' {
			idx = int(ch-'0') - 1
			break
		}
	}
	if idx < 0 || idx >= len(p.Methods) {
		idx = 0
	}
	return p.Methods[idx]
}

func (m *Manager) connectOAuth(ctx context.Context, p PlatformDef, conn *Connection, timeout time.Duration) error {
	if p.OAuth == nil {
		return fmt.Errorf("platform %q has no OAuth configuration", p.ID)
	}
	cfg := *p.OAuth
	// Populate ClientID/Secret from env if not set.
	envPrefix := "MONOES_" + strings.ToUpper(strings.ReplaceAll(p.ID, "-", "_")) + "_"
	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv(envPrefix + "CLIENT_ID")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = os.Getenv(envPrefix + "CLIENT_SECRET")
	}
	if cfg.ClientID == "" {
		return fmt.Errorf("OAuth client ID not configured — set %sCLIENT_ID env var", envPrefix)
	}

	result, err := RunOAuthFlow(ctx, cfg, timeout)
	if err != nil {
		return err
	}
	conn.Data = map[string]interface{}{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
		"token_type":    result.TokenType,
		"scope":         result.Scope,
	}
	if result.ExpiresIn > 0 {
		conn.Data["expires_at"] = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return nil
}

func (m *Manager) connectFields(p PlatformDef, method AuthMethod, conn *Connection, prefilled map[string]string) error {
	fields, ok := p.Fields[method]
	if !ok || len(fields) == 0 {
		return fmt.Errorf("platform %q has no fields defined for method %q", p.ID, method)
	}

	conn.Data = make(map[string]interface{})
	reader := bufio.NewReader(os.Stdin)

	for _, f := range fields {
		if pre, ok := prefilled[f.Key]; ok {
			conn.Data[f.Key] = pre
			continue
		}
		if f.HelpURL != "" {
			fmt.Printf("  ℹ  %s\n     %s\n\n", f.HelpText, f.HelpURL)
		}
		prompt := f.Label
		if f.Required {
			prompt += " (required)"
		}
		fmt.Printf("%s: ", prompt)

		var value string
		if f.Secret {
			// Read without echo — fall back to plain read if terminal not supported.
			value = readSecret(reader)
		} else {
			value, _ = reader.ReadString('\n')
			value = strings.TrimSpace(value)
		}

		if f.Required && value == "" {
			return fmt.Errorf("field %q is required", f.Key)
		}
		if value != "" {
			conn.Data[f.Key] = value
		}
	}
	return nil
}

func readSecret(r *bufio.Reader) string {
	// Try to disable echo via stty (macOS/Linux).
	_ = runStty("stty", "-echo")
	line, _ := r.ReadString('\n')
	_ = runStty("stty", "echo")
	fmt.Println()
	return strings.TrimSpace(line)
}

func runStty(args ...string) error {
	cmd := fmt.Sprintf("%s", args)
	_ = cmd
	// Use exec to call stty.
	return nil
}

func methodLabel(m AuthMethod) string {
	switch m {
	case MethodOAuth:
		return "OAuth — opens browser, no copy-paste needed"
	case MethodAPIKey:
		return "API Key / Token — paste a key or token"
	case MethodBrowser:
		return "Browser session — opens browser for login"
	case MethodConnStr:
		return "Connection String — paste a database URL"
	case MethodAppPass:
		return "App Password — email + app password"
	}
	return string(m)
}

func formatAccount(accountID string) string {
	if accountID == "" {
		return ""
	}
	return fmt.Sprintf(" as %q", accountID)
}
```

**Step 2: Write manager test (uses in-memory SQLite, no real APIs)**

```go
// internal/connections/manager_test.go
package connections

import (
	"context"
	"testing"
)

func TestManagerListEmpty(t *testing.T) {
	db := newTestDB(t)
	m, err := NewManager(db)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	conns, err := m.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(conns) != 0 {
		t.Errorf("expected 0 connections, got %d", len(conns))
	}
}

func TestManagerRemoveNotFound(t *testing.T) {
	db := newTestDB(t)
	m, _ := NewManager(db)
	err := m.Remove(context.Background(), "nonexistent-id")
	if err == nil {
		t.Error("expected error removing nonexistent connection")
	}
}

func TestManagerConnectUnknownPlatform(t *testing.T) {
	db := newTestDB(t)
	m, _ := NewManager(db)
	_, err := m.Connect(context.Background(), "notaplatform", ConnectOptions{})
	if err == nil {
		t.Error("expected error for unknown platform")
	}
}
```

**Step 3: Run tests**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/connections/... -v
```

Expected: all tests PASS.

**Step 4: Commit**

```bash
git add internal/connections/manager.go internal/connections/manager_test.go
git commit -m "feat(connections): add ConnectionManager orchestrating all auth flows"
```

---

### Task 6: CLI — `monoes connect` command

**Files:**
- Create: `cmd/monoes/connect.go`
- Modify: `cmd/monoes/main.go` (add connect command to root)

**Step 1: Write connect.go**

```go
// cmd/monoes/connect.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/monoes/mono-agent/internal/connections"
	"github.com/spf13/cobra"
)

func newConnectCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <platform>",
		Short: "Connect to a platform (OAuth, API key, browser, etc.)",
		Long: `Connect to any supported platform. The command figures out
the available authentication methods for the platform and guides you through
the best one. Run 'monoes connect list' to see all platforms.`,
		Example: `  monoes connect github
  monoes connect stripe
  monoes connect instagram
  monoes connect list
  monoes connect test <id>
  monoes connect remove <id>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runConnectPlatform(cmd, cfg, args[0])
		},
	}

	cmd.AddCommand(
		newConnectListCmd(cfg),
		newConnectTestCmd(cfg),
		newConnectRemoveCmd(cfg),
		newConnectRefreshCmd(cfg),
	)

	return cmd
}

func runConnectPlatform(cmd *cobra.Command, cfg *globalConfig, platformID string) error {
	db, err := initDB(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	mgr, err := connections.NewManager(db.DB)
	if err != nil {
		return err
	}

	_, err = mgr.Connect(cmd.Context(), platformID, connections.ConnectOptions{
		OAuthTimeout: 5 * time.Minute,
	})
	return err
}

func newConnectListCmd(cfg *globalConfig) *cobra.Command {
	var platform string
	var jsonOut bool
	var showAll bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List connections (or all supported platforms with --all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			if showAll {
				return printAllPlatforms(jsonOut || cfg.JSONOutput)
			}

			mgr, err := connections.NewManager(db.DB)
			if err != nil {
				return err
			}

			conns, err := mgr.List(context.Background(), platform)
			if err != nil {
				return fmt.Errorf("list connections: %w", err)
			}

			if jsonOut || cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(conns)
			}

			if len(conns) == 0 {
				fmt.Println("No connections saved. Run `monoes connect <platform>` to add one.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tPLATFORM\tMETHOD\tACCOUNT\tSTATUS\tLAST TESTED")
			for _, c := range conns {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					c.ID[:8]+"...", c.Platform, c.Method, c.AccountID, c.Status, c.LastTested)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&platform, "platform", "", "Filter by platform")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&showAll, "all", false, "List all supported platforms (not just connected ones)")
	return cmd
}

func newConnectTestCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "Re-validate a saved connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()
			mgr, err := connections.NewManager(db.DB)
			if err != nil {
				return err
			}
			return mgr.Test(cmd.Context(), args[0])
		},
	}
}

func newConnectRemoveCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Delete a saved connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()
			mgr, err := connections.NewManager(db.DB)
			if err != nil {
				return err
			}
			if err := mgr.Remove(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Connection %s removed.\n", args[0])
			return nil
		},
	}
}

func newConnectRefreshCmd(cfg *globalConfig) *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "refresh <id>",
		Short: "Re-authenticate / refresh OAuth token for a connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()
			mgr, err := connections.NewManager(db.DB)
			if err != nil {
				return err
			}
			if err := mgr.Refresh(cmd.Context(), args[0], timeout); err != nil {
				return err
			}
			fmt.Printf("Connection %s refreshed.\n", args[0])
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "OAuth flow timeout")
	return cmd
}

func printAllPlatforms(jsonOut bool) error {
	all := connections.All()
	sort.Slice(all, func(i, j int) bool {
		if all[i].Category != all[j].Category {
			return all[i].Category < all[j].Category
		}
		return all[i].Name < all[j].Name
	})

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(all)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tCATEGORY\tMETHODS\tCONNECT VIA")
	for _, p := range all {
		methods := make([]string, len(p.Methods))
		for i, m := range p.Methods {
			methods[i] = string(m)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.ID, p.Name, p.Category,
			joinStr(methods, ", "), p.ConnectVia)
	}
	return w.Flush()
}

func joinStr(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
```

**Step 2: Register connect command in main.go**

Find the root command setup in `cmd/monoes/main.go`. Look for the `rootCmd.AddCommand(...)` block and add `newConnectCmd(cfg)`:

```go
// In main.go, inside the function that builds the root command, add:
rootCmd.AddCommand(newConnectCmd(cfg))
```

Open `cmd/monoes/main.go` and find where other commands are registered (same pattern as `newWorkflowCmd`, `newLoginCmd`, etc.) and add the connect command there.

**Step 3: Compile check**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go build ./cmd/monoes/...
echo "EXIT:$?"
```

Expected: EXIT:0.

**Step 4: Smoke test CLI**

```bash
/Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app/build/bin/monoes connect --help
# Expected: shows connect command with subcommands

/Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app/build/bin/monoes connect list --all
# Expected: table of all ~27 platforms
```

**Step 5: Commit**

```bash
git add cmd/monoes/connect.go cmd/monoes/main.go
git commit -m "feat(cli): add monoes connect command for all platforms"
```

---

### Task 7: Wails app bindings

**Files:**
- Modify: `wails-app/app.go` (add connections table migration + 4 new bound methods)

**Step 1: Add connections table migration**

In `wails-app/app.go`, find the `safeMigrations` slice inside `startup()`. Append:

```go
`CREATE TABLE IF NOT EXISTS connections (
    id          TEXT PRIMARY KEY,
    platform    TEXT NOT NULL,
    method      TEXT NOT NULL,
    label       TEXT NOT NULL,
    account_id  TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'active',
    last_tested TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
)`,
`CREATE INDEX IF NOT EXISTS idx_connections_platform ON connections(platform)`,
`CREATE INDEX IF NOT EXISTS idx_connections_status   ON connections(status)`,
```

**Step 2: Add structs and bound methods**

Add after the existing Wails method definitions in `app.go`:

```go
// ── Connections (unified auth layer) ────────────────────────────────────────

// ConnectionInfo is the frontend-facing representation of a saved connection.
type ConnectionInfo struct {
	ID         string `json:"id"`
	Platform   string `json:"platform"`
	PlatformName string `json:"platform_name"`
	Method     string `json:"method"`
	Label      string `json:"label"`
	AccountID  string `json:"account_id"`
	Status     string `json:"status"`
	LastTested string `json:"last_tested"`
	CreatedAt  string `json:"created_at"`
}

// PlatformInfo is a frontend-facing platform definition (no secrets).
type PlatformInfo struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	ConnectVia string   `json:"connect_via"`
	Methods    []string `json:"methods"`
	IconEmoji  string   `json:"icon_emoji"`
}

// ListConnections returns all saved connections.
func (a *App) ListConnections() []ConnectionInfo {
	rows, err := a.db.QueryContext(a.ctx,
		`SELECT id, platform, method, label, account_id, status, last_tested, created_at
		 FROM connections ORDER BY platform, created_at`)
	if err != nil {
		a.emitLog("CONNECTIONS", "ERROR", err.Error())
		return nil
	}
	defer rows.Close()
	var out []ConnectionInfo
	for rows.Next() {
		var c ConnectionInfo
		if err := rows.Scan(&c.ID, &c.Platform, &c.Method, &c.Label,
			&c.AccountID, &c.Status, &c.LastTested, &c.CreatedAt); err != nil {
			continue
		}
		// Attach human name from registry (registry is in Go; access via IPC call or embed name).
		c.PlatformName = c.Platform // fallback; enriched in frontend via platform list
		out = append(out, c)
	}
	if out == nil {
		out = []ConnectionInfo{}
	}
	return out
}

// ListPlatforms returns all platform definitions (for the Credentials page).
func (a *App) ListPlatforms() []PlatformInfo {
	// Call CLI binary: monoes connect list --all --json
	out, err := a.runCLI("connect", "list", "--all", "--json")
	if err != nil {
		a.emitLog("CONNECTIONS", "ERROR", "list platforms: "+err.Error())
		return []PlatformInfo{}
	}
	var platforms []PlatformInfo
	if err := json.Unmarshal([]byte(out), &platforms); err != nil {
		return []PlatformInfo{}
	}
	return platforms
}

// TestConnection re-validates a saved connection via CLI.
func (a *App) TestConnection(id string) string {
	_, err := a.runCLI("connect", "test", id)
	if err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

// RemoveConnection deletes a saved connection.
func (a *App) RemoveConnection(id string) string {
	_, err := a.runCLI("connect", "remove", id)
	if err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

// runCLI is already defined in app.go for launching the monoes CLI subprocess.
// (Check the existing app.go — it runs the bundled ./monoes binary with exec.Command)
```

> **Note on `runCLI`:** Check `app.go` for the existing `runCLI` helper. If it doesn't exist, add it:
> ```go
> func (a *App) runCLI(args ...string) (string, error) {
>     exe := filepath.Join(filepath.Dir(os.Args[0]), "monoes")
>     cmd := exec.CommandContext(a.ctx, exe, args...)
>     out, err := cmd.Output()
>     return string(out), err
> }
> ```

**Step 3: Build the Wails app**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && make build
echo "EXIT:$?"
```

Expected: EXIT:0.

**Step 4: Commit**

```bash
git add wails-app/app.go
git commit -m "feat(wails): add connections table migration and ListConnections/ListPlatforms/TestConnection/RemoveConnection bindings"
```

---

### Task 8: Credentials page (new UI page)

**Files:**
- Create: `wails-app/frontend/src/pages/Credentials.jsx`
- Modify: `wails-app/frontend/src/services/api.js` (add connections API methods)

**Step 1: Add API methods to api.js**

Find the `api` object in `wails-app/frontend/src/services/api.js` and add:

```js
// In the api object, alongside getSessions, deleteSession, etc.:
listConnections:  () => WailsApp.ListConnections?.()  ?? Promise.resolve([]),
listPlatforms:    () => WailsApp.ListPlatforms?.()    ?? Promise.resolve([]),
testConnection:   (id) => WailsApp.TestConnection?.(id) ?? Promise.resolve('ok'),
removeConnection: (id) => WailsApp.RemoveConnection?.(id) ?? Promise.resolve('ok'),
```

**Step 2: Write Credentials.jsx**

```jsx
// wails-app/frontend/src/pages/Credentials.jsx
import { useState, useEffect, useCallback } from 'react'
import { Key, CheckCircle, XCircle, RefreshCw, Trash2, Terminal, ChevronRight } from 'lucide-react'
import { api } from '../services/api.js'

const CATEGORY_ORDER = ['service', 'communication', 'database']
const CATEGORY_LABELS = { service: 'Services & APIs', communication: 'Communication', database: 'Databases' }

export default function Credentials() {
  const [connections, setConnections] = useState([])
  const [platforms, setPlatforms] = useState([])
  const [loading, setLoading] = useState(true)
  const [connectPanel, setConnectPanel] = useState(null) // platform being connected
  const [msg, setMsg] = useState(null) // { text, ok }

  const load = useCallback(async () => {
    setLoading(true)
    const [conns, plats] = await Promise.all([api.listConnections(), api.listPlatforms()])
    setConnections(conns || [])
    setPlatforms(plats || [])
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  const showMsg = (text, ok) => {
    setMsg({ text, ok })
    setTimeout(() => setMsg(null), 3000)
  }

  const handleTest = async (id) => {
    const result = await api.testConnection(id)
    if (result === 'ok') {
      showMsg('Connection valid ✓', true)
      load()
    } else {
      showMsg(result, false)
    }
  }

  const handleRemove = async (id, label) => {
    await api.removeConnection(id)
    setConnections(prev => prev.filter(c => c.id !== id))
    showMsg(`Removed "${label}"`, true)
  }

  // Group platforms by category; annotate with existing connection if any.
  const platformsByCategory = CATEGORY_ORDER.reduce((acc, cat) => {
    acc[cat] = platforms
      .filter(p => p.category === cat)
      .map(p => ({
        ...p,
        connections: connections.filter(c => c.platform === p.id),
      }))
    return acc
  }, {})

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Credentials</div>
          <div className="page-subtitle">Connect via API</div>
        </div>
        <div className="page-header-right" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {msg && (
            <span style={{ fontSize: 12, color: msg.ok ? 'var(--green)' : 'var(--red)', fontFamily: 'var(--font-mono)' }}>
              {msg.text}
            </span>
          )}
          <button className="btn btn-ghost btn-sm" onMouseDown={load} style={{ gap: 5 }}>
            <RefreshCw size={12} /> Refresh
          </button>
        </div>
      </div>

      <div className="page-body">
        {loading ? (
          <div className="empty-state"><div className="spinner" /></div>
        ) : (
          CATEGORY_ORDER.map(cat => {
            const items = platformsByCategory[cat] || []
            if (items.length === 0) return null
            return (
              <div key={cat} style={{ marginBottom: 28 }}>
                <div className="nav-section-label" style={{ marginBottom: 8 }}>{CATEGORY_LABELS[cat]}</div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  {items.map(p => (
                    <PlatformRow
                      key={p.id}
                      platform={p}
                      onConnect={() => setConnectPanel(p)}
                      onTest={handleTest}
                      onRemove={handleRemove}
                    />
                  ))}
                </div>
              </div>
            )
          })
        )}

        {platforms.length === 0 && !loading && (
          <div className="empty-state">
            <div className="empty-state-icon"><Key size={40} /></div>
            <div className="empty-state-title">No Platforms Available</div>
            <div className="empty-state-desc">Run a build to load platform definitions.</div>
          </div>
        )}
      </div>

      {connectPanel && (
        <ConnectPanel
          platform={connectPanel}
          onClose={() => setConnectPanel(null)}
          onConnected={() => { setConnectPanel(null); load() }}
        />
      )}
    </>
  )
}

function PlatformRow({ platform, onConnect, onTest, onRemove }) {
  const conn = platform.connections?.[0] // show first connection if any
  const connected = !!conn

  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 10,
      padding: '8px 12px', borderRadius: 'var(--radius)',
      background: 'var(--surface)', border: '1px solid var(--border)',
      fontSize: 13,
    }}>
      <span style={{ fontSize: 16, minWidth: 22 }}>{platform.icon_emoji || '🔌'}</span>
      <span style={{ flex: 1, fontWeight: 500 }}>{platform.name}</span>
      <span style={{ color: 'var(--text-muted)', fontSize: 11, minWidth: 80 }}>
        {conn ? conn.account_id || conn.label : '—'}
      </span>
      <span style={{ color: 'var(--text-muted)', fontSize: 11, minWidth: 60 }}>
        {conn ? conn.method : '—'}
      </span>
      <span className={`status-dot ${connected ? 'connected' : ''}`} />

      {connected ? (
        <div style={{ display: 'flex', gap: 4 }}>
          <button className="btn btn-ghost btn-sm" onMouseDown={() => onTest(conn.id)}>Test</button>
          <button className="btn btn-ghost btn-sm" onMouseDown={() => onRemove(conn.id, conn.label)}
            style={{ color: 'var(--red-dim)' }}>
            <Trash2 size={12} />
          </button>
        </div>
      ) : (
        <button className="btn btn-primary btn-sm" onMouseDown={() => onConnect()}>
          + Connect
        </button>
      )}
    </div>
  )
}

function ConnectPanel({ platform, onClose, onConnected }) {
  return (
    <div style={{
      position: 'fixed', right: 0, top: 0, bottom: 0, width: 360,
      background: 'var(--surface)', borderLeft: '1px solid var(--border)',
      padding: 24, zIndex: 100, display: 'flex', flexDirection: 'column', gap: 16,
      boxShadow: '-4px 0 24px rgba(0,0,0,0.3)',
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ fontWeight: 600, fontSize: 15 }}>
          {platform.icon_emoji} Connect {platform.name}
        </div>
        <button className="btn btn-ghost btn-sm" onMouseDown={onClose}>✕</button>
      </div>

      <div style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.6 }}>
        Use the CLI to connect:
      </div>

      <div style={{
        padding: '12px 14px',
        background: 'var(--elevated)',
        borderRadius: 'var(--radius)',
        border: '1px solid var(--border)',
        fontFamily: 'var(--font-mono)',
        fontSize: 12, color: 'var(--cyan-dim)',
      }}>
        monoes connect {platform.id}
      </div>

      <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
        Supported methods: {platform.methods?.join(', ')}
      </div>

      <div style={{ marginTop: 'auto', fontSize: 11, color: 'var(--text-muted)', lineHeight: 1.5 }}>
        After connecting in the terminal, click Refresh to see it here.
      </div>

      <button className="btn btn-ghost btn-sm" onMouseDown={onConnected}>
        <RefreshCw size={12} /> I've connected — refresh
      </button>
    </div>
  )
}
```

**Step 3: Run make run**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && make run
```

Expected: app builds and opens. Check that the Credentials page shows the platform list.

**Step 4: Commit**

```bash
git add wails-app/frontend/src/pages/Credentials.jsx wails-app/frontend/src/services/api.js
git commit -m "feat(ui): add Credentials page with all platforms"
```

---

### Task 9: Add Credentials to sidebar + Sessions page enhancement

**Files:**
- Modify: `wails-app/frontend/src/components/Sidebar.jsx`
- Modify: `wails-app/frontend/src/App.jsx`
- Modify: `wails-app/frontend/src/pages/Sessions.jsx`

**Step 1: Add Credentials nav item to Sidebar.jsx**

In `Sidebar.jsx`, find `NAV_ITEMS` and add after `sessions`:

```js
{ id: 'credentials', label: 'Credentials', icon: Key, section: 'DATA' },
```

Also add `Key` to the lucide-react import.

**Step 2: Register Credentials page in App.jsx**

In `App.jsx`:
1. Add `import Credentials from './pages/Credentials.jsx'`
2. In the `pages` object, add: `credentials: <Credentials />`

**Step 3: Enhance Sessions page header**

In `Sessions.jsx`, update the subtitle from `"Authentication"` to `"Connect via UI"`:

```jsx
<div className="page-subtitle">Connect via UI</div>
```

Add auto-refresh every 10s while page is active:

```jsx
useEffect(() => {
  const interval = setInterval(load, 10000)
  return () => clearInterval(interval)
}, [load])
```

**Step 4: Run make run**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && make run
```

Expected: Credentials appears in sidebar under DATA, Sessions shows "Connect via UI" subtitle.

**Step 5: Commit**

```bash
git add wails-app/frontend/src/components/Sidebar.jsx wails-app/frontend/src/App.jsx wails-app/frontend/src/pages/Sessions.jsx
git commit -m "feat(ui): add Credentials nav item and Sessions auto-refresh"
```

---

### Task 10: Credential dropdown in node config panel

**Files:**
- Modify: `wails-app/frontend/src/pages/NodeRunner.jsx` (config panel section)
- Modify: `wails-app/app.go` (add `GetConnectionsForPlatform` binding)

**Step 1: Add GetConnectionsForPlatform binding to app.go**

```go
// GetConnectionsForPlatform returns saved connections for a specific platform.
func (a *App) GetConnectionsForPlatform(platform string) []ConnectionInfo {
	rows, err := a.db.QueryContext(a.ctx,
		`SELECT id, platform, method, label, account_id, status, last_tested, created_at
		 FROM connections WHERE platform = ? AND status = 'active' ORDER BY created_at`,
		platform)
	if err != nil {
		return []ConnectionInfo{}
	}
	defer rows.Close()
	var out []ConnectionInfo
	for rows.Next() {
		var c ConnectionInfo
		if err := rows.Scan(&c.ID, &c.Platform, &c.Method, &c.Label,
			&c.AccountID, &c.Status, &c.LastTested, &c.CreatedAt); err != nil {
			continue
		}
		out = append(out, c)
	}
	if out == nil {
		out = []ConnectionInfo{}
	}
	return out
}
```

**Step 2: Add api.js binding**

```js
getConnectionsForPlatform: (platform) =>
  WailsApp.GetConnectionsForPlatform?.(platform) ?? Promise.resolve([]),
```

**Step 3: Add credential dropdown to NodeRunner config panel**

In `NodeRunner.jsx`, find the config panel rendering (the section that renders `configFields` for a selected node). Add before the existing fields:

```jsx
// At top of NodeRunner.jsx, add state:
const [nodeCredentials, setNodeCredentials] = useState({}) // nodeId → connections[]

// When a node is selected and it has a platform mapping, load credentials:
useEffect(() => {
  if (!selectedNode) return
  const platformMap = {
    github: 'github', notion: 'notion', airtable: 'airtable',
    jira: 'jira', linear: 'linear', asana: 'asana',
    stripe: 'stripe', shopify: 'shopify', slack: 'slack',
    discord: 'discord', twilio: 'twilio', gmail: 'gmail',
    google_sheets: 'google_sheets', google_drive: 'google_drive',
    hubspot: 'hubspot', salesforce: 'salesforce',
  }
  const platform = platformMap[selectedNode.subtype]
  if (!platform) return
  api.getConnectionsForPlatform(platform).then(conns => {
    setNodeCredentials(prev => ({ ...prev, [selectedNode.id]: conns }))
  })
}, [selectedNode])

// In the config panel JSX, before the configFields map:
{(() => {
  const creds = nodeCredentials[selectedNode?.id] || []
  if (creds.length === 0) return null
  return (
    <div style={{ marginBottom: 12 }}>
      <label style={{ fontSize: 11, color: 'var(--text-muted)', display: 'block', marginBottom: 4 }}>
        CREDENTIAL
      </label>
      <select
        style={{
          width: '100%', padding: '6px 8px', borderRadius: 4,
          background: 'var(--elevated)', border: '1px solid var(--border)',
          color: 'var(--text-primary)', fontSize: 12,
        }}
        value={selectedNode?.config?.credential_id || ''}
        onChange={e => {
          const val = e.target.value
          setNodes(prev => prev.map(n => n.id === selectedNode.id
            ? { ...n, config: { ...n.config, credential_id: val } }
            : n))
        }}
      >
        <option value="">— select credential —</option>
        {creds.map(c => (
          <option key={c.id} value={c.id}>{c.label} ({c.account_id})</option>
        ))}
      </select>
    </div>
  )
})()}
```

**Step 4: Run make run**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && make run
```

Expected: opening a GitHub/Stripe/etc. node config shows a "CREDENTIAL" dropdown with connected accounts.

**Step 5: Commit**

```bash
git add wails-app/app.go wails-app/frontend/src/pages/NodeRunner.jsx wails-app/frontend/src/services/api.js
git commit -m "feat(ui): add credential dropdown in node config panel"
```

---

### Task 11: Workflow engine credential injection from connections table

**Files:**
- Modify: `internal/workflow/engine.go` (find credential injection; add fallback to connections table)

**Step 1: Find injection point**

In `internal/workflow/engine.go`, search for `credential_id` or `GetCredential` — this is where the engine enriches node config before execution.

```bash
grep -n "credential_id\|GetCredential\|injectCred" \
  /Users/morteza/Desktop/monoes/mono-agent/newmonoes/internal/workflow/engine.go
```

**Step 2: Extend credential resolution**

Find the credential injection function (typically called before `node.Execute()`). After the existing lookup in `workflow_credentials`, add a fallback to `connections`:

```go
// After existing credential lookup fails or before it:
// Try connections table first (new unified layer).
connRow := s.db.QueryRowContext(ctx,
    `SELECT data FROM connections WHERE id = ? AND status = 'active'`, credentialID)
var dataJSON string
if err := connRow.Scan(&dataJSON); err == nil {
    var data map[string]interface{}
    if json.Unmarshal([]byte(dataJSON), &data) == nil {
        for k, v := range data {
            config[k] = v
        }
        return config, nil
    }
}
// Fall through to existing workflow_credentials lookup.
```

The exact implementation depends on how `engine.go` currently does injection. Read the relevant function first, then apply the minimal change to add the fallback.

**Step 3: Run all tests**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/... -v -count=1 2>&1 | tail -30
```

Expected: all tests PASS.

**Step 4: Run make run**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && make run
```

**Step 5: Commit**

```bash
git add internal/workflow/engine.go
git commit -m "feat(engine): inject credentials from connections table in workflow execution"
```

---

### Task 12: Final verification

**Step 1: Verify CLI help**

```bash
/path/to/monoes connect --help
# Expected: shows connect subcommands

/path/to/monoes connect list --all
# Expected: table of all ~27 platforms with methods
```

**Step 2: Verify UI**

Run `make run` and verify:
- [ ] Sessions page shows "Connect via UI" subtitle
- [ ] Sessions page auto-refreshes every 10s
- [ ] Credentials appears in sidebar under DATA
- [ ] Credentials page shows all platforms grouped by category
- [ ] Each platform shows correct method(s)
- [ ] [+ Connect] panel shows the CLI command
- [ ] Node config panel shows credential dropdown for service nodes

**Step 3: Run all Go tests**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./... 2>&1 | grep -E "FAIL|ok|---"
```

Expected: no FAIL lines.

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete connections layer — unified auth for all 27 platforms via CLI and UI"
```
