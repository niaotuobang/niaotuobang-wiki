package wiki

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyAccountsAndDeletedArchivesMigration(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	sender := &captureSender{}
	a, err := Open(Config{DataDir: t.TempDir(), Auth: &AuthConfig{Secret: key, Email: sender}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	bundle := map[string]any{
		"users":  []any{map[string]any{"id": 1, "name": "原管理员", "email": "owner@example.test", "about": "保留简介", "created_at": "2020-01-01 01:00:00", "updated_at": "2020-01-02 01:00:00"}, map[string]any{"id": 2, "name": "旧署名甲", "email": "shared@example.test"}, map[string]any{"id": 3, "name": "旧署名乙", "email": "SHARED@example.test"}},
		"topics": []any{map[string]any{"id": 1, "name": "人物", "description": "原分类"}},
		"articles": []any{
			map[string]any{"id": 7, "title": "现存文章", "body": "<div>当前内容</div>", "topic_id": 1, "user_id": 2, "created_at": "2020-01-01T00:00:00Z", "updated_at": "2021-01-01T00:00:00Z", "history": []any{map[string]any{"archive_id": 11, "title": "原始标题", "body": "<div>历史内容</div>", "user_id": 3, "topic_unknown": true, "created_at": "2020-01-01T00:00:00Z", "updated_at": "2020-01-01 00:00:00"}}},
			map[string]any{"id": 8, "archive_id": 12, "title": "已删除文章", "body": "<div>仍保留原文</div>", "topic_id": "", "user_id": 99, "created_at": "2020-01-01T00:00:00Z", "updated_at": "2020-01-01T00:00:00Z", "deleted_at": "2026-01-01T00:00:00Z", "deletion_time_unknown": true, "creation_time_unknown": true, "topic_unknown": true},
		}}
	raw, _ := json.Marshal(bundle)
	input := filepath.Join(t.TempDir(), "export.json")
	os.WriteFile(input, raw, 0600)
	r, err := a.ImportMatterwikiAccounts(input, "1", false)
	if err != nil || r.Applied || r.Users != 3 || r.Identities != 2 || r.SharedEmailAliases != 1 {
		t.Fatalf("preview: %+v %v", r, err)
	}
	var count int
	a.store.DB.QueryRow(`SELECT count(*) FROM users`).Scan(&count)
	if count != 0 {
		t.Fatal("preview created users")
	}
	if _, err = a.ImportMatterwikiAccounts(input, "1", true); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ImportMatterwiki(input, true); err != nil {
		t.Fatal(err)
	}
	v, err := getArticle(a.store.DB, "legacy-7")
	if err != nil || v.AuthorID != "legacy-user-3" || v.UpdatedBy != "legacy-user-2" {
		t.Fatalf("author attribution: %+v %v", v, err)
	}
	var cipher []byte
	var canonical, about string
	a.store.DB.QueryRow(`SELECT email_cipher,canonical_user_id,about FROM legacy_user_profiles WHERE user_id='legacy-user-1'`).Scan(&cipher, &canonical, &about)
	address, err := a.auth.decrypt("email", cipher)
	if err != nil || address != "owner@example.test" || about != "保留简介" {
		t.Fatal("profile not preserved or encrypted")
	}
	a.store.DB.QueryRow(`SELECT canonical_user_id FROM legacy_user_profiles WHERE user_id='legacy-user-3'`).Scan(&canonical)
	if canonical != "legacy-user-2" {
		t.Fatal("shared mailbox lost original alias")
	}
	c := &testClient{t: t, app: a}
	proof := c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": "owner@example.test"}, 202)
	login := c.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": proof["challenge_id"].(string), "code": sender.last().code}, 200)
	if login["user"].(map[string]any)["name"] != "原管理员" || login["user"].(map[string]any)["role"] != "system_admin" {
		t.Fatal("passwordless migrated administrator login failed")
	}
	c.call("GET", "/api/v1/articles/legacy-8", nil, 404)
	trash := c.call("GET", "/api/v1/admin/trash", nil, 200)["items"].([]any)
	if len(trash) != 1 || trash[0].(map[string]any)["deletion_time_unknown"] != true {
		t.Fatal("deleted archive not preserved as unknown-time trash")
	}
	c.call("GET", "/api/v1/admin/trash/legacy-8/versions/1", nil, 200)
	a.store.DB.QueryRow(`SELECT count(*) FROM legacy_revision_map`).Scan(&count)
	if count != 2 {
		t.Fatal("archive IDs were lost")
	}
	a.store.DB.QueryRow(`SELECT count(*) FROM users WHERE length(password_hash)>0`).Scan(&count)
	if count != 0 {
		t.Fatal("passwords imported")
	}
	if _, err = a.ImportMatterwikiAccounts(input, "1", true); err != nil {
		t.Fatal("account retry with placeholder user failed:", err)
	}
	repeated, err := a.ImportMatterwiki(input, true)
	if err != nil || repeated.Skipped != 2 {
		t.Fatal("article migration not idempotent")
	}
}
