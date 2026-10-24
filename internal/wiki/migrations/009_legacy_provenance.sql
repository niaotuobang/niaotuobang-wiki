CREATE TABLE legacy_user_profiles(
 user_id TEXT PRIMARY KEY REFERENCES users(id),
 about TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT '',
 updated_at TEXT NOT NULL DEFAULT '',
 email_cipher BLOB,
 canonical_user_id TEXT REFERENCES users(id)
);
CREATE TABLE legacy_revision_map(
 legacy_archive_id TEXT PRIMARY KEY,
 article_id TEXT NOT NULL REFERENCES articles(id),
 revision INTEGER NOT NULL,
 original_updated_at TEXT NOT NULL,
 body_sha256 TEXT NOT NULL,
 UNIQUE(article_id,revision)
);
CREATE TABLE legacy_article_metadata(
 article_id TEXT PRIMARY KEY REFERENCES articles(id),
 deletion_time_unknown INTEGER NOT NULL DEFAULT 0,
 creation_time_unknown INTEGER NOT NULL DEFAULT 0
);
