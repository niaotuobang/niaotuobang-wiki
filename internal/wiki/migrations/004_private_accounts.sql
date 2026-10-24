-- Preserve the legacy role column for old imports; access_role is authoritative.
ALTER TABLE users ADD COLUMN access_role TEXT NOT NULL DEFAULT 'user' CHECK(access_role IN ('system_admin','admin','user'));
ALTER TABLE users ADD COLUMN can_edit INTEGER NOT NULL DEFAULT 0;
UPDATE users SET access_role=CASE role WHEN 'admin' THEN 'admin' ELSE 'user' END,can_edit=CASE role WHEN 'viewer' THEN 0 ELSE 1 END;
ALTER TABLE sessions ADD COLUMN reauthenticated_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
DELETE FROM sessions;
CREATE TABLE IF NOT EXISTS invites(token_hash TEXT PRIMARY KEY,role TEXT NOT NULL,expires_at TEXT NOT NULL);
ALTER TABLE invites ADD COLUMN created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE invites ADD COLUMN revoked_at TEXT NOT NULL DEFAULT '';
CREATE TABLE user_identities(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),kind TEXT NOT NULL CHECK(kind IN ('phone','email')),address_cipher BLOB NOT NULL,lookup_hash TEXT NOT NULL,verified_at TEXT NOT NULL,UNIQUE(kind,lookup_hash),UNIQUE(user_id,kind));
CREATE TABLE auth_challenges(id TEXT PRIMARY KEY,kind TEXT NOT NULL,purpose TEXT NOT NULL,lookup_hash TEXT NOT NULL,address_cipher BLOB NOT NULL,subject TEXT NOT NULL,user_id TEXT NOT NULL DEFAULT '',invite_hash TEXT NOT NULL DEFAULT '',code_hash TEXT NOT NULL,expires_at TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,state TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE INDEX auth_challenges_target ON auth_challenges(lookup_hash,created_at);
CREATE TABLE auth_rate_limits(key TEXT PRIMARY KEY,count INTEGER NOT NULL,expires_at TEXT NOT NULL);
CREATE TABLE auth_audit_events(id TEXT PRIMARY KEY,actor_id TEXT NOT NULL,event TEXT NOT NULL,object_id TEXT NOT NULL,result TEXT NOT NULL,created_at TEXT NOT NULL);
UPDATE settings SET value='false' WHERE key='public_read';
