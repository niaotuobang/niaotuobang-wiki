package wiki

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpdateNicknamePreservesIdentityAndHistory(t *testing.T) {
	a, client := newTestApp(t)
	before := client.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	article := client.call("POST", "/api/v1/articles", publishBody("昵称更新测试"), 201)
	id := article["id"].(string)
	client.call("PATCH", "/api/v1/account/profile", map[string]string{"nickname": "  新昵称  "}, 200)
	after := client.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	if after["name"] != "新昵称" || before["id"] != after["id"] || before["role"] != after["role"] || before["login"] != after["login"] {
		t.Fatal("nickname update changed identity or permissions")
	}
	current := client.call("GET", "/api/v1/articles/"+id, nil, 200)
	if current["author_name"] != "新昵称" || current["updated_by_name"] != "新昵称" || current["revision"] != article["revision"] {
		t.Fatal("current attribution did not reflect nickname or revision changed")
	}
	history := client.call("GET", "/api/v1/articles/"+id+"/versions/1", nil, 200)
	if history["updated_by_name"] != before["name"] {
		t.Fatal("changing nickname rewrote historical snapshot")
	}
	members := client.call("GET", "/api/v1/community", nil, 200)["items"].([]any)
	if members[0].(map[string]any)["name"] != "新昵称" {
		t.Fatal("community name not updated")
	}
	var audits int
	if err := a.store.DB.QueryRow(`SELECT count(*) FROM auth_audit_events WHERE actor_id=? AND event='account.profile.update'`, before["id"]).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("profile audit missing", err)
	}
}

func TestNicknamePermissionsAndValidation(t *testing.T) {
	a, _ := newTestApp(t)
	viewer := newUserClient(t, a, "viewer")
	viewer.call("PATCH", "/api/v1/account/profile", map[string]string{"nickname": "只读成员的新昵称"}, 200)
	for _, name := range []string{"", "   ", strings.Repeat("字", 41), "两\n行", "含\x00字符"} {
		viewer.call("PATCH", "/api/v1/account/profile", map[string]string{"nickname": name}, 400)
	}
	viewer.call("PATCH", "/api/v1/account/profile", map[string]string{"nickname": "名字", "id": "another-user"}, 400)
	response := viewer.raw("PATCH", "/api/v1/account/profile", bytes.NewBufferString(`{"nickname":"名字"}`), false)
	if response.Code != 403 {
		t.Fatal("profile update accepted without CSRF token")
	}
	anon := &testClient{t: t, app: a}
	anon.call("PATCH", "/api/v1/account/profile", map[string]string{"nickname": "名字"}, 401)
	user := viewer.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	if _, err := a.store.DB.Exec(`UPDATE users SET disabled=1 WHERE id=?`, user["id"]); err != nil {
		t.Fatal(err)
	}
	viewer.call("PATCH", "/api/v1/account/profile", map[string]string{"nickname": "名字"}, 401)
}
