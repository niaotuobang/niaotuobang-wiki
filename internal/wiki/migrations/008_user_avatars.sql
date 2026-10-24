ALTER TABLE users ADD COLUMN avatar_media_id TEXT REFERENCES media(id);
