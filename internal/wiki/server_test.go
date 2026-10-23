package wiki

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishingDraftsConflictsAndChineseSearch(t *testing.T) {
	app, err := Open(Config{DataDir: t.TempDir(), Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	var cookie *http.Cookie
	request := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
			r.Header.Set("X-CSRF-Token", csrfToken(cookie.Value))
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		for _, c := range w.Result().Cookies() {
			cookie = c
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	request("POST", "/api/v1/auth/demo", map[string]any{}, 200)
	tag := request("POST", "/api/v1/tags", map[string]any{"name": "测试人物", "description": "测试", "show_in_nav": true}, 201)
	body := map[string]any{"title": "林砚的小传", "document": Node{Type: "doc", Content: []Node{{Type: "paragraph", Content: []Node{{Type: "text", Text: "两字昵称林砚也能被搜索。"}}}}}, "tag_ids": []string{tag["id"].(string)}, "occurred_at": "2025-05", "date_precision": "month", "status": "draft", "expected_revision": 0}
	a := request("POST", "/api/v1/articles", body, 201)
	id := a["id"].(string)
	result := request("GET", "/api/v1/articles?q=林砚", nil, 200)
	if len(result["items"].([]any)) != 0 {
		t.Fatal("draft leaked into search")
	}
	body["status"] = "published"
	a = request("PUT", "/api/v1/articles/"+id, body, 200)
	if a["revision"] != float64(1) {
		t.Fatal("first publication must be revision 1")
	}
	result = request("GET", "/api/v1/articles?q=林砚", nil, 200)
	if len(result["items"].([]any)) != 1 {
		t.Fatal("Chinese substring search missing article")
	}
	request("PUT", "/api/v1/articles/"+id, body, 409)
	body["expected_revision"] = 1
	body["title"] = "林砚的新故事"
	request("PUT", "/api/v1/drafts/"+id, body, 200)
	a = request("GET", "/api/v1/articles/"+id, nil, 200)
	if a["title"] != "林砚的小传" {
		t.Fatal("autosave changed published text")
	}
	request("PUT", "/api/v1/articles/"+id, body, 200)
	versions := request("GET", "/api/v1/articles/"+id+"/versions", nil, 200)
	if len(versions["items"].([]any)) != 2 {
		t.Fatal("publication history missing")
	}
	request("POST", "/api/v1/articles/"+id+"/versions/1/restore", map[string]any{"expected_revision": 2}, 200)
	a = request("GET", "/api/v1/articles/"+id, nil, 200)
	if a["revision"] != float64(3) || a["title"] != "林砚的小传" {
		t.Fatal("restore must create a new version")
	}
	request("DELETE", "/api/v1/articles/"+id, nil, 200)
	request("GET", "/api/v1/articles/"+id, nil, 404)
	request("POST", "/api/v1/articles/"+id+"/restore", map[string]any{}, 200)
}

func TestContentValidation(t *testing.T) {
	for _, n := range []Node{
		{Type: "doc", Content: []Node{{Type: "script", Text: "unsafe"}}},
		{Type: "doc", Content: []Node{{Type: "paragraph", Content: []Node{{Type: "text", Text: "link", Marks: []Mark{{Type: "link", Attrs: map[string]any{"href": "javascript:alert(1)"}}}}}}}},
	} {
		if _, err := validateDocument(n); err == nil {
			t.Fatal("unsafe document accepted")
		}
	}
	if err := validateDate("2025-02-30", "day"); err == nil {
		t.Fatal("invalid calendar date accepted")
	}
	if err := validateDate("2025", "year"); err != nil {
		t.Fatal(err)
	}
}
