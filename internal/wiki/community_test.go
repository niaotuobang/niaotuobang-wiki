package wiki

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommunityActivityCountsAndPrivacy(t *testing.T) {
	a, client := newTestApp(t)
	addUser := func(id, name, created string, disabled bool) {
		t.Helper()
		_, err := a.store.DB.Exec(`INSERT INTO users(id,name,login,password_hash,role,created_at,disabled) VALUES(?,?,?,'','viewer',?,?)`, id, name, id, created, disabled)
		if err != nil {
			t.Fatal(err)
		}
	}
	addUser("author", "故事作者", "2020-01-01T00:00:00Z", false)
	addUser("editor", "最近修改者", "2020-01-01T00:00:00Z", false)
	addUser("previous", "早期修改者", "2020-01-01T00:00:00Z", false)
	addUser("newer", "新群友", "2024-01-01T00:00:00Z", false)
	addUser("older", "老群友", "2023-01-01T00:00:00Z", false)
	addUser("hidden", "已禁用", "2099-01-01T00:00:00Z", true)
	addUser("legacy-user-99", "缺失作者占位", "2099-01-01T00:00:00Z", false)
	a.store.DB.Exec(`UPDATE users SET created_at='1999-01-01T00:00:00Z' WHERE access_role='system_admin'`)
	addArticle := func(id, author, editor, date, status, deleted string) {
		t.Helper()
		_, err := a.store.DB.Exec(`INSERT INTO articles(id,title,document,plain_text,status,author_id,updated_by,created_at,updated_at,deleted_at) VALUES(?,?,'{}','',?,?,?,'2025-01-01T00:00:00Z',?,?)`, id, id, status, author, editor, date, deleted)
		if err != nil {
			t.Fatal(err)
		}
	}
	addArticle("shared", "author", "editor", "2025-03-01T00:00:00Z", "published", "")
	for revision, date := range []string{"2025-02-01T00:00:00Z", "2025-02-02T00:00:00Z"} {
		snapshot, _ := json.Marshal(map[string]string{"updated_by": "previous", "updated_at": date})
		if _, err := a.store.DB.Exec(`INSERT INTO article_versions(article_id,revision,snapshot) VALUES('shared',?,?)`, revision+1, string(snapshot)); err != nil {
			t.Fatal(err)
		}
	}
	addArticle("private", "newer", "newer", "2099-01-01T00:00:00Z", "draft", "")
	addArticle("recycled", "older", "older", "2099-01-01T00:00:00Z", "published", "2025-01-02T00:00:00Z")
	anon := httptest.NewRecorder()
	a.Handler().ServeHTTP(anon, httptest.NewRequest("GET", "/api/v1/community", nil))
	if anon.Code != 401 {
		t.Fatal("anonymous access to member wall accepted")
	}
	result := client.call("GET", "/api/v1/community", nil, 200)
	items := result["items"].([]any)
	want := []string{"editor", "previous", "author", "newer", "older"}
	if len(items) != 6 {
		t.Fatalf("unexpected visible member count: %d", len(items))
	}
	for i, id := range want {
		member := items[i].(map[string]any)
		if member["id"] != id {
			t.Fatalf("position %d: got %v want %s", i, member["id"], id)
		}
		count := float64(0)
		if i < 3 {
			count = 1
		}
		if member["article_count"] != count {
			t.Fatalf("%s article count: %v", id, member["article_count"])
		}
	}
	for _, item := range items {
		for key := range item.(map[string]any) {
			if key != "id" && key != "name" && key != "avatar_media_id" && key != "article_count" {
				t.Fatalf("private member field exposed: %s", key)
			}
		}
	}
	// A fractional-second edit must move its editor to the front immediately.
	_, err := a.store.DB.Exec(`UPDATE articles SET updated_by='previous',updated_at='2025-03-01T00:00:00.100Z' WHERE id='shared'`)
	if err != nil {
		t.Fatal(err)
	}
	result = client.call("GET", "/api/v1/community", nil, 200)
	if result["items"].([]any)[0].(map[string]any)["id"] != "previous" {
		t.Fatal("modifying an article did not update member order")
	}
	// Equal edit times use registration time, not a random user ID.
	_, err = a.store.DB.Exec(`UPDATE users SET created_at='2024-01-01T00:00:00Z' WHERE id='author'`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.store.DB.Exec(`UPDATE articles SET updated_by='author',updated_at='2025-02-02T00:00:00Z' WHERE id='shared'`)
	if err != nil {
		t.Fatal(err)
	}
	result = client.call("GET", "/api/v1/community", nil, 200)
	if result["items"].([]any)[0].(map[string]any)["id"] != "author" {
		t.Fatal("equal activity did not use newest registration first")
	}
}

func TestCommunityPreservesRegistrationTimes(t *testing.T) {
	a, _ := newTestApp(t)
	for _, id := range []string{"legacy-user-7", "joined-user"} {
		if _, err := a.store.DB.Exec(`INSERT INTO users(id,name,login,password_hash,role) VALUES(?,?,?,'','viewer')`, id, id, id); err != nil {
			t.Fatal(err)
		}
	}
	a.store.DB.Exec(`INSERT INTO legacy_user_profiles(user_id,created_at) VALUES('legacy-user-7','2020-01-02 03:04:05')`)
	a.store.DB.Exec(`INSERT INTO auth_audit_events VALUES('join-event','joined-user','auth.join','joined-user','success','2025-01-02T03:04:05Z')`)
	// Apply the backfill to a schema with the new column already present.
	sql, err := migrationFiles.ReadFile("migrations/010_user_creation_time.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.DB.Exec(strings.SplitN(string(sql), ";", 2)[1]); err != nil {
		t.Fatal(err)
	}
	for id, expected := range map[string]string{"legacy-user-7": "2020-01-02 03:04:05", "joined-user": "2025-01-02T03:04:05Z"} {
		var actual string
		if err := a.store.DB.QueryRow(`SELECT created_at FROM users WHERE id=?`, id).Scan(&actual); err != nil || actual != expected {
			t.Fatalf("registration time for %s: %q, %v", id, actual, err)
		}
	}
}
