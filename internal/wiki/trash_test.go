package wiki

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"testing"
)

func TestTrashMediaReadableByAdministratorOnly(t *testing.T) {
	a, admin := newTestApp(t)
	author := newUserClient(t, a, "editor")
	reader := newUserClient(t, a, "viewer")
	var payload bytes.Buffer
	if err := png.Encode(&payload, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	intent := author.call("POST", "/api/v1/media/uploads", map[string]any{"name": "photo.png", "content_type": "image/png", "size": payload.Len()}, 201)
	mid := intent["media"].(map[string]any)["id"].(string)
	if w := author.raw("PUT", intent["url"].(string), bytes.NewReader(payload.Bytes()), true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	author.call("POST", "/api/v1/media/uploads/"+mid+"/complete", nil, 200)
	body := publishBody("回收站中的合照")
	doc := document("合照仍然保留")
	doc.Content = append(doc.Content, Node{Type: "image", Attrs: map[string]any{"mediaId": mid}})
	body["document"] = doc
	id := author.call("POST", "/api/v1/articles", body, 201)["id"].(string)
	admin.call("DELETE", "/api/v1/articles/"+id, nil, 200)
	if w := admin.raw("GET", "/media/"+mid, nil, true); w.Code != 200 {
		t.Fatalf("admin cannot view trash media: %d", w.Code)
	}
	if w := reader.raw("GET", "/media/"+mid, nil, true); w.Code != 404 {
		t.Fatalf("trash media leaked to ordinary reader: %d", w.Code)
	}
	if w := (&testClient{t: t, app: a}).raw("GET", "/media/"+mid, nil, false); w.Code != 401 {
		t.Fatalf("trash media leaked anonymously: %d", w.Code)
	}
}

func TestTrashPreservesVersionEditorsAndRestrictsAccess(t *testing.T) {
	a, admin := newTestApp(t)
	editor := newUserClient(t, a, "user")
	creator := admin.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"]
	modifier := editor.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"]
	v := admin.call("POST", "/api/v1/articles", publishBody("多人补充的故事"), 201)
	id := v["id"].(string)
	body := publishBody("第二版")
	body["expected_revision"] = 1
	body["change_summary"] = "补充了新的故事"
	editor.call("PUT", "/api/v1/articles/"+id, body, 200)
	admin.call("DELETE", "/api/v1/articles/"+id, nil, 200)
	for _, path := range []string{"/api/v1/articles/" + id, "/api/v1/articles/" + id + "/versions", "/api/v1/articles/" + id + "/versions/1"} {
		admin.call("GET", path, nil, 404)
	}
	for _, path := range []string{"/api/v1/admin/trash", "/api/v1/admin/trash/" + id, "/api/v1/admin/trash/" + id + "/versions", "/api/v1/admin/trash/" + id + "/versions/1"} {
		editor.call("GET", path, nil, 403)
		(&testClient{t: t, app: a}).call("GET", path, nil, 401)
	}
	admin.call("GET", "/api/v1/admin/trash/"+id, nil, 200)
	versions := admin.call("GET", "/api/v1/admin/trash/"+id+"/versions", nil, 200)["items"].([]any)
	if len(versions) != 2 || versions[0].(map[string]any)["updated_by"] != modifier || versions[1].(map[string]any)["updated_by"] != creator {
		t.Fatal("version editors not preserved")
	}
	first := admin.call("GET", "/api/v1/admin/trash/"+id+"/versions/1", nil, 200)
	if first["title"] != "多人补充的故事" || first["updated_by"] != creator {
		t.Fatal("wrong historical content or editor")
	}
	admin.call("POST", "/api/v1/articles/"+id+"/restore", nil, 200)
	admin.call("GET", "/api/v1/admin/trash/"+id, nil, 404)
	admin.call("GET", "/api/v1/articles/"+id+"/versions/1", nil, 200)
	// A different administrator must not gain access to another user's draft.
	draft := publishBody("仅作者可见的草稿")
	draft["status"] = "draft"
	d := editor.call("POST", "/api/v1/articles", draft, 201)["id"].(string)
	editor.call("DELETE", "/api/v1/articles/"+d, nil, 200)
	admin.call("GET", "/api/v1/admin/trash/"+d, nil, 404)
	if len(admin.call("GET", "/api/v1/admin/trash", nil, 200)["items"].([]any)) != 0 {
		t.Fatal("private draft leaked into trash list")
	}
}

func TestTrashPaginationWithEqualDeletionTimes(t *testing.T) {
	a, c := newTestApp(t)
	for i := 0; i < 21; i++ {
		v := c.call("POST", "/api/v1/articles", publishBody(fmt.Sprintf("故事 %d", i)), 201)
		c.call("DELETE", "/api/v1/articles/"+v["id"].(string), nil, 200)
	}
	if _, err := a.store.DB.Exec(`UPDATE articles SET deleted_at='2026-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	page := c.call("GET", "/api/v1/admin/trash", nil, 200)
	items := page["items"].([]any)
	if len(items) != 20 || page["next_cursor"] == "" {
		t.Fatal("missing first page or cursor")
	}
	next := c.call("GET", "/api/v1/admin/trash?cursor="+url.QueryEscape(page["next_cursor"].(string)), nil, 200)
	last := next["items"].([]any)
	if len(last) != 1 || next["next_cursor"] != "" {
		t.Fatal("pagination lost records")
	}
	for _, item := range items {
		if item.(map[string]any)["id"] == last[0].(map[string]any)["id"] {
			t.Fatal("duplicate article")
		}
	}
	c.call("GET", "/api/v1/admin/trash?cursor=invalid", nil, 400)
}
