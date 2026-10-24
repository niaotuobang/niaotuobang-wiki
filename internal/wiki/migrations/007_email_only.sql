ALTER TABLE invites ADD COLUMN recovery_user_id TEXT NOT NULL DEFAULT '';
UPDATE users SET password_hash=X'';
DELETE FROM sessions;
DELETE FROM auth_challenges;
