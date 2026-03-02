-- 001_initial.sql
-- Initial schema for monoes-agent storage layer.

-- crawler_sessions
CREATE TABLE IF NOT EXISTS crawler_sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL,
    platform      TEXT NOT NULL,
    cookies_json  TEXT NOT NULL,
    expiry        TIMESTAMP NOT NULL,
    when_added    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    profile_photo BLOB,
    UNIQUE(username, platform)
);
CREATE INDEX IF NOT EXISTS idx_sessions_platform ON crawler_sessions(platform);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON crawler_sessions(expiry);

-- actions
CREATE TABLE IF NOT EXISTS actions (
    id                     TEXT PRIMARY KEY,
    created_at             INTEGER NOT NULL,
    title                  TEXT NOT NULL,
    type                   TEXT NOT NULL,
    state                  TEXT NOT NULL DEFAULT 'PENDING',
    disabled               INTEGER NOT NULL DEFAULT 0,
    target_platform        TEXT NOT NULL,
    position               INTEGER DEFAULT 0,
    content_subject        TEXT,
    content_message        TEXT,
    content_blob_urls      TEXT,
    scheduled_date         TEXT,
    execution_interval     INTEGER,
    start_date             TEXT,
    end_date               TEXT,
    campaign_id            TEXT,
    reached_index          INTEGER DEFAULT 0,
    keywords               TEXT,
    action_execution_count INTEGER DEFAULT 0,
    created_at_ts          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at_ts          TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_actions_state ON actions(state);
CREATE INDEX IF NOT EXISTS idx_actions_type ON actions(type);
CREATE INDEX IF NOT EXISTS idx_actions_platform ON actions(target_platform);

-- people (must come before action_targets due to FK)
CREATE TABLE IF NOT EXISTS people (
    id                TEXT PRIMARY KEY,
    platform_username TEXT NOT NULL,
    platform          TEXT NOT NULL,
    full_name         TEXT,
    image_url         TEXT,
    contact_details   TEXT,
    website           TEXT,
    content_count     INTEGER DEFAULT 0,
    follower_count    TEXT,
    following_count   INTEGER DEFAULT 0,
    introduction      TEXT,
    is_verified       INTEGER DEFAULT 0,
    category          TEXT,
    job_title         TEXT,
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(platform_username, platform)
);
CREATE INDEX IF NOT EXISTS idx_people_username ON people(platform_username);
CREATE INDEX IF NOT EXISTS idx_people_platform ON people(platform);
CREATE INDEX IF NOT EXISTS idx_people_name ON people(full_name);

-- action_targets
CREATE TABLE IF NOT EXISTS action_targets (
    id                 TEXT PRIMARY KEY,
    action_id          TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
    person_id          TEXT REFERENCES people(id),
    platform           TEXT NOT NULL,
    link               TEXT,
    source_type        TEXT,
    status             TEXT NOT NULL DEFAULT 'PENDING',
    last_interacted_at TEXT,
    comment_text       TEXT,
    metadata           TEXT,
    created_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_targets_action ON action_targets(action_id);
CREATE INDEX IF NOT EXISTS idx_targets_person ON action_targets(person_id);
CREATE INDEX IF NOT EXISTS idx_targets_status ON action_targets(status);
CREATE INDEX IF NOT EXISTS idx_targets_platform ON action_targets(platform);

-- social_lists
CREATE TABLE IF NOT EXISTS social_lists (
    id         TEXT PRIMARY KEY,
    list_type  TEXT,
    name       TEXT NOT NULL,
    item_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_lists_name ON social_lists(name);

-- social_list_items
CREATE TABLE IF NOT EXISTS social_list_items (
    id                TEXT PRIMARY KEY,
    list_id           TEXT NOT NULL REFERENCES social_lists(id) ON DELETE CASCADE,
    platform          TEXT NOT NULL,
    platform_username TEXT NOT NULL,
    image_url         TEXT,
    url               TEXT,
    full_name         TEXT,
    contact_details   TEXT,
    follower_count    INTEGER DEFAULT 0,
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_list_items_list ON social_list_items(list_id);
CREATE INDEX IF NOT EXISTS idx_list_items_platform ON social_list_items(platform);
CREATE INDEX IF NOT EXISTS idx_list_items_username ON social_list_items(platform_username);

-- threads
CREATE TABLE IF NOT EXISTS threads (
    id             TEXT PRIMARY KEY,
    social_user_id TEXT NOT NULL,
    platform       TEXT NOT NULL,
    metadata       TEXT,
    messages       TEXT,
    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(social_user_id, platform)
);
CREATE INDEX IF NOT EXISTS idx_threads_user ON threads(social_user_id);

-- templates
CREATE TABLE IF NOT EXISTS templates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    subject    TEXT,
    body       TEXT NOT NULL,
    metadata   TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name);

-- configs
CREATE TABLE IF NOT EXISTS configs (
    name        TEXT PRIMARY KEY,
    config_data TEXT NOT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- settings
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- schema_migrations
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
