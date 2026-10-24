package wiki

import (
	"net/http/httptest"
	"testing"
)

func TestRandomArticleExcludesCurrentAndPrivateContent(t *testing.T) {
	a, client := newTestApp(t)
	var uid string
	if err := a.store.DB.QueryRow(`SELECT id FROM users WHERE access_role='system_admin'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, status, deleted string }{
		{"current", "published", ""}, {"other", "published", ""},
		{"private", "draft", ""}, {"recycled", "published", "2025-01-01T00:00:00Z"},
	} {
		_, err := a.store.DB.Exec(`INSERT INTO articles(id,title,document,plain_text,status,author_id,updated_by,created_at,updated_at,deleted_at) VALUES(?,?,'{}','',?,?,?,'2025-01-01T00:00:00Z','2025-01-01T00:00:00Z',?)`, item.id, item.id, item.status, uid, uid, item.deleted)
		if err != nil {
			t.Fatal(err)
		}
	}
	result := client.call("GET", "/api/v1/articles/random?exclude=current", nil, 200)
	if result["id"] != "other" {
		t.Fatalf("random article did not exclude current/private/recycled content: %v", result)
	}
	if _, err := a.store.DB.Exec(`UPDATE articles SET deleted_at='2025-01-01T00:00:00Z' WHERE id='other'`); err != nil {
		t.Fatal(err)
	}
	client.call("GET", "/api/v1/articles/random?exclude=current", nil, 404)
	if result := client.call("GET", "/api/v1/articles/random", nil, 200); result["id"] != "current" {
		t.Fatal("sidebar random entry should still select the only readable article")
	}
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/articles/random", nil))
	if response.Code != 401 {
		t.Fatal("anonymous access to random article accepted")
	}
}
