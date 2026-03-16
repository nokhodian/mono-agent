-- data/migrations/007_posts_comments.sql

CREATE TABLE IF NOT EXISTS posts (
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
);

CREATE TABLE IF NOT EXISTS post_comments (
  id          TEXT PRIMARY KEY,
  post_id     TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  author      TEXT NOT NULL,
  text        TEXT,
  timestamp   TEXT,
  likes_count INTEGER DEFAULT 0,
  reply_count INTEGER DEFAULT 0,
  scraped_at  TEXT NOT NULL,
  UNIQUE(post_id, author, timestamp)
);
