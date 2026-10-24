package wiki

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLegacyAccountsUpgradePreservesAuthorshipAndReadOnly(t *testing.T) {
	dir := t.TempDir()
	db, e := sql.Open("sqlite", filepath.Join(dir, "wiki.db"))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range []string{"001_initial.sql", "002_link_context.sql", "003_member_access.sql"} {
		b, e := migrationFiles.ReadFile("migrations/" + f)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(e)
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("old-test-password"), bcrypt.MinCost)
	db.Exec(`INSERT INTO users(id,name,login,password_hash,role) VALUES('old-admin','管理员','old-admin',?,'admin'),('old-reader','阅读者','old-reader',?,'viewer')`, hash, hash)
	db.Exec(`INSERT INTO sessions(token_hash,user_id,expires_at) VALUES('old-token','old-admin','2099-01-01T00:00:00Z')`)
	db.Exec(`PRAGMA user_version=3`)
	db.Close()
	a, e := Open(Config{DataDir: dir})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if e = a.RequireSystemAdmin(); e == nil {
		t.Fatal("upgrade silently selected an owner")
	}
	if e = a.PromoteSystemAdmin("old-reader"); e == nil {
		t.Fatal("reader became system administrator")
	}
	if e = a.PromoteSystemAdmin("old-admin"); e != nil {
		t.Fatal(e)
	}
	u, e := userByID(a.store.DB, "old-reader")
	if e != nil || u.Role != "user" || u.CanEdit {
		t.Fatal("legacy read-only permission lost")
	}
	var sessions int
	a.store.DB.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions)
	if sessions != 0 {
		t.Fatal("old sessions survived migration")
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "migration-backups", "*.db"))
	if len(backups) != 1 {
		t.Fatal("migration backup missing")
	}
	if e = a.RequireSystemAdmin(); e != nil {
		t.Fatal(e)
	}
}
func TestIdentityBackupRestoresOnlyWithSeparateKey(t *testing.T) {
	a, c, s := identityTestApp(t)
	p := c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "bind", "target": "restore@example.com"}, 202)
	c.call("POST", "/api/v1/account/identities/bind", map[string]string{"challenge_id": p["challenge_id"].(string), "code": s.last().code}, 200)
	archive, e := a.Backup(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(t.TempDir(), "restored")
	if e = RestoreBackup(archive, dir); e != nil {
		t.Fatal(e)
	}
	bad := make([]byte, 32)
	if b, e := Open(Config{DataDir: dir, Auth: &AuthConfig{Secret: bad}}); e == nil {
		b.Close()
		t.Fatal("wrong separate key accepted")
	}
	b, e := Open(Config{DataDir: dir, Auth: &AuthConfig{Secret: a.auth.cfg.Secret}})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	var cipher []byte
	if e = b.store.DB.QueryRow(`SELECT address_cipher FROM user_identities LIMIT 1`).Scan(&cipher); e != nil {
		t.Fatal(e)
	}
	target, e := b.auth.decrypt("email", cipher)
	if e != nil || target != "restore@example.com" {
		t.Fatal("identity restoration failed")
	}
	for _, table := range []string{"sessions", "invites", "auth_challenges"} {
		var n int
		b.store.DB.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n)
		if n != 0 {
			t.Fatal("backup retained reusable authentication state", table)
		}
	}
}
func TestLocalSystemAdministratorRecoveryRevokesSessions(t *testing.T) {
	a, c, s := identityTestApp(t)
	u := c.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	path, e := a.PrepareEmailRecovery(u["id"].(string), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	link, e := url.Parse(strings.TrimSpace(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	token := link.Query().Get("invite")
	c.call("GET", "/api/v1/articles", nil, 401)
	c.call("POST", "/api/v1/auth/login", map[string]string{"login": "admin", "password": "only-for-local-tests"}, 410)
	guest := &testClient{t: t, app: a}
	v := guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "join", "target": "recovered@example.com", "invite": token}, 202)
	guest.call("POST", "/api/v1/auth/join", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code, "invite": token, "nickname": "恢复管理员"}, 200)
	recovered := guest.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	if recovered["id"] != u["id"] || recovered["role"] != "system_admin" {
		t.Fatal("recovery changed owner identity")
	}
	(&testClient{t: t, app: a}).call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "join", "target": "second@example.com", "invite": token}, 403)
}
