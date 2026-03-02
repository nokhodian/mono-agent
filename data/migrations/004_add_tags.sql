-- 004_add_tags.sql
-- Global tag registry and per-person tag associations.

CREATE TABLE IF NOT EXISTS tags (
    id    TEXT PRIMARY KEY,
    name  TEXT NOT NULL UNIQUE COLLATE NOCASE,
    color TEXT NOT NULL DEFAULT '#00b4d8'
);

CREATE TABLE IF NOT EXISTS people_tags (
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    tag_id    TEXT NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
    PRIMARY KEY (person_id, tag_id)
);
