-- 005_add_action_params.sql
-- Add params column to actions table for per-action custom parameter storage.
ALTER TABLE actions ADD COLUMN params TEXT NOT NULL DEFAULT '{}';
