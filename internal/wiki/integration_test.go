package wiki

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testClient struct {
	t      *testing.T
	app    *App
	cookie *http.Cookie
}

func (c *testClient) call(method, path string, body any, status int) map[string]any {
	c.t.Helper()
	b, _ := json.Marshal(body)
	w := c.raw(method, path, bytes.NewReader(b), true)
	if w.Code != status {
		c.t.Fatalf("%s %s: %d, expected %d: %s", method, path, w.Code, status, w.Body.String())
	}
	var v map[string]any
	json.Unmarshal(w.Body.Bytes(), &v)
	return v
}
func (c *testClient) raw(method, path string, body io.Reader, csrf bool) *httptest.ResponseRecorder {
	c.t.Helper()
	r := httptest.NewRequest(method, path, body)
	r.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		r.AddCookie(c.cookie)
		if csrf {
			r.Header.Set("X-CSRF-Token", csrfToken(c.cookie.Value))
		}
	}
	w := httptest.NewRecorder()
	c.app.Handler().ServeHTTP(w, r)
	for _, cookie := range w.Result().Cookies() {
		c.cookie = cookie
	}
	return w
}
func newTestApp(t *testing.T) (*App, *testClient) {
	t.Helper()
	a, e := Open(Config{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	c := &testClient{t: t, app: a}
	key := make([]byte, 32)
	rand.Read(key)
	sender := &captureSender{}
	if e := a.initAuth(AuthConfig{Secret: key, Email: sender, Phone: sender}); e != nil {
		t.Fatal(e)
	}
	u, e := a.createUser("测试管理员", "admin", "only-for-local-tests", "system_admin")
	if e != nil {
		t.Fatal(e)
	}
	a.setupToken = ""
	w := httptest.NewRecorder()
	if e = a.login(w, u); e != nil {
		t.Fatal(e)
	}
	c.cookie = w.Result().Cookies()[0]
	return a, c
}
func newUserClient(t *testing.T, a *App, role string) *testClient {
	t.Helper()
	login := newID()
	u, e := a.createUser("群友", login, "test-password-123", role)
	if e != nil {
		t.Fatal(e)
	}
	c := &testClient{t: t, app: a}
	w := httptest.NewRecorder()
	if e = a.login(w, u); e != nil {
		t.Fatal(e)
	}
	c.cookie = w.Result().Cookies()[0]
	return c
}
func publishBody(title string) map[string]any {
	return map[string]any{"title": title, "document": document("一个测试故事"), "status": "published", "expected_revision": 0}
}
func TestPermissionsAndCSRF(t *testing.T) {
	a, admin := newTestApp(t)
	anon := &testClient{t: t, app: a}
	viewer := newUserClient(t, a, "viewer")
	editor := newUserClient(t, a, "editor")
	anon.call("GET", "/api/v1/articles", nil, 401)
	viewer.call("POST", "/api/v1/articles", publishBody("不应发布"), 403)
	draft := admin.call("POST", "/api/v1/articles", map[string]any{"title": "秘密草稿", "document": document("不可泄露"), "status": "draft"}, 201)
	id := draft["id"].(string)
	editor.call("GET", "/api/v1/articles/"+id, nil, 404)
	editor.call("GET", "/api/v1/drafts/"+id, nil, 404)
	if w := admin.raw("POST", "/api/v1/tags", strings.NewReader(`{"name":"CSRF"}`), false); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://untrusted.invalid")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
	admin.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "other@example.com", "setup_token": "bad"}, 403)
	editor.call("POST", "/api/v1/admin/invites", map[string]string{"role": "editor"}, 403)
	invite := admin.call("POST", "/api/v1/admin/invites", map[string]string{"role": "editor"}, 201)
	member := &testClient{t: t, app: a}
	b := map[string]any{"invite": invite["invite"], "name": "新群友", "login": "new-member", "password": "new-password-123"}
	member.call("POST", "/api/v1/auth/register", b, 410)
	b["login"] = "another-member"
	anon.call("POST", "/api/v1/auth/register", b, 410)
	member = newUserClient(t, a, "editor")
	admin.call("GET", "/api/v1/admin", nil, 200)
	admin.call("PATCH", "/api/v1/admin/members/"+admin.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"].(string), map[string]any{"role": "viewer", "disabled": false}, 409)
	me := member.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	admin.call("PATCH", "/api/v1/admin/members/"+me["id"].(string), map[string]any{"role": "editor", "disabled": true}, 200)
	member.call("GET", "/api/v1/articles", nil, 401)
}
func TestMediaOwnershipRangeVersionsAndBackup(t *testing.T) {
	a, c := newTestApp(t)
	other := newUserClient(t, a, "editor")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var payload bytes.Buffer
	png.Encode(&payload, img)
	intent := c.call("POST", "/api/v1/media/uploads", map[string]any{"name": "测试.png", "content_type": "image/png", "size": payload.Len()}, 201)
	mid := intent["media"].(map[string]any)["id"].(string)
	other.call("POST", "/api/v1/media/uploads/"+mid+"/complete", map[string]any{}, 403)
	c.call("POST", "/api/v1/media/uploads/"+mid+"/complete", map[string]any{}, 400)
	if w := c.raw("PUT", intent["url"].(string), bytes.NewReader(payload.Bytes()), true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	c.call("POST", "/api/v1/media/uploads/"+mid+"/complete", map[string]any{}, 200)
	c.call("POST", "/api/v1/media/uploads/"+mid+"/complete", map[string]any{}, 200)
	b := publishBody("有合照的故事")
	doc := document("那天的合照")
	doc.Content = append(doc.Content, Node{Type: "image", Attrs: map[string]any{"mediaId": mid, "alt": "合照"}})
	b["document"] = doc
	other.call("POST", "/api/v1/articles", b, 403)
	article := c.call("POST", "/api/v1/articles", b, 201)
	id := article["id"].(string)
	r := httptest.NewRequest("GET", "/media/"+mid, nil)
	r.AddCookie(other.cookie)
	r.Header.Set("Range", "bytes=0-7")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), payload.Bytes()[:8]) {
		t.Fatalf("range failed: %d", w.Code)
	}
	b["document"] = document("移走正文图片，历史版本仍然需要它")
	b["expected_revision"] = 1
	other.call("PUT", "/api/v1/articles/"+id, b, 200)
	if w := other.raw("GET", "/media/"+mid, nil, true); w.Code != 200 {
		t.Fatal("historical media became inaccessible")
	}
	backup, e := a.Backup(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if e = RestoreBackup(backup, target); e != nil {
		t.Fatal(e)
	}
	restored, e := Open(Config{DataDir: target})
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	v, e := getArticle(restored.store.DB, id)
	if e != nil || v.Revision != 2 {
		t.Fatal("article restore failed", e)
	}
	m, e := restored.media(mid)
	if e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(target, m.Key))
	if e != nil || !bytes.Equal(data, payload.Bytes()) {
		t.Fatal("restored media differs", e)
	}
	var sessions int
	restored.store.DB.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions)
	if sessions != 0 {
		t.Fatal("backup restored active sessions")
	}
	if e = RestoreBackup(backup, target); e == nil {
		t.Fatal("restore overwrote existing data")
	}
}
func TestBacklinksTagIntersectionAndDeletedTagRestore(t *testing.T) {
	a, c := newTestApp(t)
	t1 := c.call("POST", "/api/v1/tags", map[string]any{"name": "人物"}, 201)
	t2 := c.call("POST", "/api/v1/tags", map[string]any{"name": "结婚"}, 201)
	target := c.call("POST", "/api/v1/articles", publishBody("阿砚"), 201)
	targetID := target["id"].(string)
	b := publishBody("那场婚礼")
	d := document("一个故事")
	d.Content[0].Content[0].Marks = []Mark{{Type: "link", Attrs: map[string]any{"href": "/articles/" + targetID}}}
	b["document"] = d
	b["tag_ids"] = []any{t1["id"], t2["id"]}
	source := c.call("POST", "/api/v1/articles", b, 201)
	sid := source["id"].(string)
	result := c.call("GET", "/api/v1/articles?tag="+t1["id"].(string)+"&tag="+t2["id"].(string), nil, 200)
	if result["total"] != float64(1) {
		t.Fatal("intersection failed")
	}
	links := c.call("GET", "/api/v1/articles/"+targetID+"/backlinks", nil, 200)
	if len(links["items"].([]any)) != 1 {
		t.Fatal("missing backlink")
	}
	c.call("DELETE", "/api/v1/tags/"+t1["id"].(string), nil, 200)
	c.call("POST", "/api/v1/articles/"+sid+"/versions/1/restore", map[string]any{"expected_revision": 1}, 422)
	c.call("POST", "/api/v1/articles/"+sid+"/versions/1/restore", map[string]any{"expected_revision": 1, "ignore_missing_tags": true}, 200)
	c.call("DELETE", "/api/v1/articles/"+sid, nil, 200)
	links = c.call("GET", "/api/v1/articles/"+targetID+"/backlinks", nil, 200)
	if len(links["items"].([]any)) != 0 {
		t.Fatal("trash leaked through backlinks")
	}
	_ = a
}
