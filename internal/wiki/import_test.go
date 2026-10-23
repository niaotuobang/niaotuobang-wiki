package wiki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMatterwikiImportPreservesProseHistoryAndIsIdempotent(t *testing.T) {
	a, _ := newTestApp(t)
	dir := t.TempDir()
	input := filepath.Join(dir, "export.json")
	bundle := map[string]any{"topics": []any{map[string]any{"id": 1, "name": "人物", "description": "小传"}}, "users": []any{map[string]any{"id": 7, "name": "旧站作者"}}, "articles": []any{map[string]any{"id": 80, "title": "阿砚", "body": "<p>保留人工写下的<strong>原文</strong>。</p><p>第二段。</p>", "topic_id": 1, "user_id": 7, "created_at": "2024-01-02T00:00:00Z", "updated_at": "2025-03-04T00:00:00Z", "history": []any{map[string]any{"body": "<p>最早的小传。</p>", "title": "阿砚初稿", "user_id": 7, "topic_id": 1, "created_at": "2024-01-02T00:00:00Z"}}}}}
	b, _ := json.Marshal(bundle)
	if e := os.WriteFile(input, b, 0600); e != nil {
		t.Fatal(e)
	}
	report, e := a.ImportMatterwiki(input, false)
	if e != nil {
		t.Fatal(e)
	}
	if report.Articles != 1 {
		t.Fatal("incorrect preview")
	}
	var count int
	a.store.DB.QueryRow(`SELECT count(*) FROM articles`).Scan(&count)
	if count != 0 {
		t.Fatal("preview wrote articles")
	}
	report, e = a.ImportMatterwiki(input, true)
	if e != nil {
		t.Fatal(e)
	}
	v, e := getArticle(a.store.DB, "legacy-80")
	if e != nil {
		t.Fatal(e)
	}
	if v.AuthorName != "旧站作者" || v.Revision != 2 || len(v.Tags) != 1 || v.Tags[0].Name != "人物" {
		t.Fatalf("lost metadata: %#v", v)
	}
	if v.PlainText != "保留人工写下的原文。\n第二段。" {
		t.Fatalf("lost prose: %q", v.PlainText)
	}
	if v.UpdatedAt != "2025-03-04T00:00:00Z" {
		t.Fatal("original time lost")
	}
	old, e := a.version("legacy-80", "1")
	if e != nil || old.Title != "阿砚初稿" {
		t.Fatal("history missing", e)
	}
	report, e = a.ImportMatterwiki(input, true)
	if e != nil || report.Skipped != 1 {
		t.Fatal("repeat import was not skipped", e)
	}
	a.store.DB.QueryRow(`SELECT count(*) FROM articles`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate import")
	}
}
