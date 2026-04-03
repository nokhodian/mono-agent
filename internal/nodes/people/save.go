package peoplenodes

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/monoes/monoes-agent/internal/bot"
	_ "github.com/monoes/monoes-agent/internal/bot/instagram"
	_ "github.com/monoes/monoes-agent/internal/bot/linkedin"
	_ "github.com/monoes/monoes-agent/internal/bot/tiktok"
	_ "github.com/monoes/monoes-agent/internal/bot/x"
	"github.com/monoes/monoes-agent/internal/util"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// globalPeopleDB is the SQLite DB used by PeopleSaveNode. Set at startup.
var globalPeopleDB *sql.DB

// SetGlobalPeopleDB wires the shared SQLite connection into all PeopleSaveNode instances.
func SetGlobalPeopleDB(db *sql.DB) {
	globalPeopleDB = db
}

// RegisterAll registers all people node types into the registry.
func RegisterAll(r *workflow.NodeTypeRegistry, db *sql.DB) {
	SetGlobalPeopleDB(db)
	r.Register("people.save", func() workflow.NodeExecutor { return &PeopleSaveNode{} })
}

// PeopleSaveNode upserts input items into the people SQLite table.
// Type: "people.save"
//
// Each input item should have at least one of:
//   - profile_url / url  — the person's profile URL (used to extract username)
//   - full_name          — display name
//   - platform           — overridden by the node config's "platform" field if set
//
// Items without a resolvable username are skipped.
type PeopleSaveNode struct{}

func (n *PeopleSaveNode) Type() string { return "people.save" }

func (n *PeopleSaveNode) Execute(
	ctx context.Context,
	input workflow.NodeInput,
	config map[string]interface{},
) ([]workflow.NodeOutput, error) {
	if globalPeopleDB == nil {
		return nil, fmt.Errorf("people.save: database not available (call SetGlobalPeopleDB at startup)")
	}

	// Config-level platform override.
	configPlatform, _ := config["platform"].(string)

	tx, err := globalPeopleDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("people.save: begin tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit

	stmt, err := tx.PrepareContext(ctx,
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
	)
	if err != nil {
		return nil, fmt.Errorf("people.save: prepare stmt: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	var savedItems []workflow.Item

	for _, item := range input.Items {
		data := item.JSON

		// Resolve platform (config override > item field).
		platform := configPlatform
		if platform == "" {
			platform, _ = data["platform"].(string)
		}
		platformUpper := strings.ToUpper(platform)

		// Resolve profile URL (prefer explicit profile_url, fall back to url or href).
		profileURL := firstString(data, "profile_url", "url", "href")

		// Extract username from profile URL via platform adapter.
		username := ""
		if profileURL != "" {
			if factory, ok := bot.PlatformRegistry[platformUpper]; ok {
				username = factory().ExtractUsername(profileURL)
			}
			if username == "" {
				// Generic fallback: last path segment.
				parts := strings.Split(strings.Trim(profileURL, "/"), "/")
				if len(parts) > 0 {
					username = strings.TrimPrefix(parts[len(parts)-1], "@")
				}
			}
		}
		if username == "" {
			continue // Cannot save without a username.
		}

		fullName, _ := data["full_name"].(string)
		imageURL, _ := data["image_url"].(string)
		website, _ := data["website"].(string)
		jobTitle := firstString(data, "job_title", "position", "headline")
		introduction, _ := data["introduction"].(string)
		isVerified, _ := data["is_verified"].(bool)

		followerCount := toNumericString(data, "follower_count", "followers_count", "followersCount")
		followingCount := toNumericString(data, "following_count")
		contentCount := toNumericString(data, "content_count")

		var followerInt, followingInt, contentInt int64
		if followerCount != "" {
			followerInt, _ = util.ConvertAbbreviatedNumber(stripWordSuffix(followerCount))
		}
		if followingCount != "" {
			followingInt, _ = util.ConvertAbbreviatedNumber(stripWordSuffix(followingCount))
		}
		if contentCount != "" {
			contentInt, _ = util.ConvertAbbreviatedNumber(stripWordSuffix(contentCount))
		}

		_, err := stmt.ExecContext(ctx,
			uuid.New().String(),
			username,
			platformUpper,
			nullableStr(fullName),
			nullableStr(imageURL),
			nil, // contact_details
			nullableStr(website),
			nullableInt(contentInt),
			nullableInt(followerInt),
			nullableInt(followingInt),
			nullableStr(introduction),
			isVerified,
			nil, // category
			nullableStr(jobTitle),
			nullableStr(profileURL),
			now,
			now,
		)
		if err != nil {
			return nil, fmt.Errorf("people.save: upsert %s/%s: %w", platformUpper, username, err)
		}

		// Emit the saved item enriched with resolved username.
		out := make(map[string]interface{}, len(data)+2)
		for k, v := range data {
			out[k] = v
		}
		out["platform_username"] = username
		out["platform"] = platformUpper
		if profileURL != "" {
			out["profile_url"] = profileURL
		}
		savedItems = append(savedItems, workflow.NewItem(out))
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("people.save: commit: %w", err)
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: savedItems},
	}, nil
}

// firstString returns the first non-empty string value found under the given keys.
func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// toNumericString returns a string value for the first matching key.
func toNumericString(m map[string]interface{}, keys ...string) string {
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

// stripWordSuffix removes trailing words like "followers", "posts" from count strings.
func stripWordSuffix(s string) string {
	parts := strings.Fields(s)
	if len(parts) > 0 {
		return parts[0]
	}
	return s
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
