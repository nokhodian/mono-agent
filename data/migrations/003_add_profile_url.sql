-- 003_add_profile_url.sql
-- Add profile_url column to people table to store the social media profile page URL.

ALTER TABLE people ADD COLUMN profile_url TEXT;
