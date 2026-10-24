ALTER TABLE users ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
UPDATE users SET created_at=coalesce(
 (SELECT nullif(p.created_at,'') FROM legacy_user_profiles p WHERE p.user_id=users.id),
 (SELECT min(e.created_at) FROM auth_audit_events e WHERE e.object_id=users.id AND e.event IN ('auth.join','auth.allowlist_join','auth.setup') AND e.result='success'),
 ''
);
