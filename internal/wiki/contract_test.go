package wiki

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticDeepLinksAndMissingAssets(t *testing.T) {
	a, c := newTestApp(t)
	a.cfg.UI = fstest.MapFS{"index.html": {Data: []byte("<!doctype html><div id=app></div>")}, "assets/main-abcd.js": {Data: []byte("console.log('wiki')")}}
	for _, path := range []string{"/", "/articles/example", "/articles/example/edit", "/timeline", "/tags/example"} {
		w := c.raw("GET", path, nil, true)
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
			t.Fatal("deep link failed", path)
		}
	}
	w := c.raw("GET", "/assets/main-abcd.js", nil, true)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("asset cache missing")
	}
	w = c.raw("GET", "/assets/missing.js", nil, true)
	if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing asset was cached or fell back to HTML")
	}
	w = c.raw("GET", "/api/v1/missing", nil, true)
	if w.Code != 404 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("API errors must be JSON")
	}
	w = c.raw("GET", "/not-a-page", nil, true)
	if w.Code != 404 {
		t.Fatal("unknown route fell back to SPA")
	}
}
func TestDemoCannotUseCloudStorage(t *testing.T) {
	if _, e := Open(Config{DataDir: filepath.Join(t.TempDir(), "unused"), Demo: true, OSS: &OSSConfig{Endpoint: "https://example.invalid"}}); e == nil {
		t.Fatal("demo inherited cloud storage configuration")
	}
}
func TestSearchRankingCursorAndDraftDate(t *testing.T) {
	a, c := newTestApp(t)
	title := c.call("POST", "/api/v1/articles", publishBody("林砚的小传"), 201)
	body := publishBody("一次聊天")
	body["document"] = document(strings.Repeat("前文。", 100) + "林砚在末尾出现")
	c.call("POST", "/api/v1/articles", body, 201)
	page, e := a.store.list(url.Values{"q": []string{"林砚"}, "limit": []string{"1"}})
	if e != nil {
		t.Fatal(e)
	}
	items := page["items"].([]Article)
	if len(items) != 1 || items[0].ID != title["id"] {
		t.Fatal("title match must rank first")
	}
	second, e := a.store.list(url.Values{"q": []string{"林砚"}, "limit": []string{"1"}, "cursor": []string{page["next_cursor"].(string)}})
	if e != nil {
		t.Fatal(e)
	}
	results := second["items"].([]Article)
	if len(results) != 1 || !strings.Contains(results[0].PlainText, "林砚") || second["next_cursor"] != "" {
		t.Fatal("search cursor or contextual excerpt failed")
	}
	draft := publishBody("")
	draft["status"] = "draft"
	draft["occurred_at"] = "202"
	draft["date_precision"] = "year"
	article := c.call("POST", "/api/v1/articles", draft, 201)
	draft["title"] = "日期还没写完"
	draft["status"] = "published"
	c.call("PUT", "/api/v1/articles/"+article["id"].(string), draft, 400)
}
func TestConcurrentEditorsAndLosslessExport(t *testing.T) {
	a, c := newTestApp(t)
	article := c.call("POST", "/api/v1/articles", publishBody("一起写的故事"), 201)
	id := article["id"].(string)
	u, _ := a.session(requestWithCookie(c.cookie))
	input := WriteArticle{Title: "新补充", Document: document("同一个基础版本"), Status: "published", ExpectedRevision: 1}
	done := make(chan error, 2)
	for range 2 {
		go func() { _, e := a.store.saveArticle(id, input, *u, false); done <- e }()
	}
	success, conflict := 0, 0
	for range 2 {
		e := <-done
		if e == nil {
			success++
		} else {
			var api *APIError
			if errors.As(e, &api) && api.Status == 409 {
				conflict++
			} else {
				t.Fatal(e)
			}
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("one edit must win and the other must conflict")
	}
	if input.Document.Content[0].Attrs["id"] == "" {
		t.Fatal("fixture paragraph ID missing")
	}
	var out bytes.Buffer
	if e := a.ExportJSON(&out); e != nil {
		t.Fatal(e)
	}
	var exported map[string]any
	if e := json.Unmarshal(out.Bytes(), &exported); e != nil {
		t.Fatal(e)
	}
	if len(exported["versions"].([]any)) != 2 {
		t.Fatal("export dropped version history")
	}
	folder := t.TempDir()
	if e := a.ExportFolder(folder); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(folder, "articles.md")); e != nil {
		t.Fatal(e)
	}
}

func requestWithCookie(cookie *http.Cookie) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookie)
	return r
}
