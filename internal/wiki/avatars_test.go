package wiki

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func avatarPayload(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, color.RGBA{uint8(20 * x), uint8(30 * y), 100, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func uploadTestAvatar(t *testing.T, c *testClient) string {
	t.Helper()
	w := c.raw("POST", "/api/v1/account/avatar", bytes.NewReader(avatarPayload(t)), true)
	if w.Code != 200 {
		t.Fatalf("avatar upload: %d %s", w.Code, w.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response["avatar_media_id"]
}
func TestReadOnlyMemberAvatarPermissionsAndBackup(t *testing.T) {
	a, admin := newTestApp(t)
	viewer := newUserClient(t, a, "viewer")
	mid := uploadTestAvatar(t, viewer)
	me := viewer.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	if me["avatar_media_id"] != mid || me["can_edit"] != false {
		t.Fatal("avatar missing or read-only role changed")
	}
	viewer.call("POST", "/api/v1/articles", publishBody("仍不可编辑"), 403)
	w := admin.raw("GET", "/media/"+mid, nil, true)
	if w.Code != 200 {
		t.Fatal("other member cannot see avatar")
	}
	imageCfg, format, err := image.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil || format != "png" || imageCfg.Width != 256 || imageCfg.Height != 256 {
		t.Fatal("avatar not normalized to square PNG")
	}
	if w := (&testClient{t: t, app: a}).raw("GET", "/media/"+mid, nil, false); w.Code != 401 {
		t.Fatal("avatar publicly readable")
	}
	if w := viewer.raw("POST", "/api/v1/account/avatar", bytes.NewReader(avatarPayload(t)), false); w.Code != 403 {
		t.Fatal("avatar update missing CSRF protection")
	}
	backup, err := a.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "restored")
	if err := RestoreBackup(backup, dir); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var restoredID, key string
	if err := store.DB.QueryRow(`SELECT u.avatar_media_id,m.object_key FROM users u JOIN media m ON m.id=u.avatar_media_id WHERE u.id=?`, me["id"]).Scan(&restoredID, &key); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, key))
	if err != nil || restoredID != mid || !bytes.Equal(data, w.Body.Bytes()) {
		t.Fatal("avatar lost during backup restore")
	}
	viewer.call("DELETE", "/api/v1/account/avatar", nil, 200)
	if avatar := viewer.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["avatar_media_id"]; avatar != nil && avatar != "" {
		t.Fatal("avatar not cleared")
	}
	if w := admin.raw("GET", "/media/"+mid, nil, true); w.Code != 404 {
		t.Fatal("unused avatar remained visible to other members")
	}
}
func TestAvatarsRejectInvalidAndOversizedImages(t *testing.T) {
	_, c := newTestApp(t)
	for _, b := range [][]byte{[]byte("<svg><script>bad</script></svg>"), {}, []byte("not an image")} {
		if w := c.raw("POST", "/api/v1/account/avatar", bytes.NewReader(b), true); w.Code != 400 {
			t.Fatalf("invalid avatar accepted: %d", w.Code)
		}
	}
	if w := c.raw("POST", "/api/v1/account/avatar", bytes.NewReader(make([]byte, (2<<20)+1)), true); w.Code != 413 {
		t.Fatalf("oversized avatar accepted: %d", w.Code)
	}
}
func TestHistoricVersionsUseCurrentAvatarWithoutChangingEditor(t *testing.T) {
	_, c := newTestApp(t)
	id := c.call("POST", "/api/v1/articles", publishBody("头像更新不会改变修改人"), 201)["id"].(string)
	before := c.call("GET", "/api/v1/articles/"+id+"/versions/1", nil, 200)
	mid := uploadTestAvatar(t, c)
	after := c.call("GET", "/api/v1/articles/"+id+"/versions/1", nil, 200)
	if after["updated_by_avatar"] != mid || after["updated_by"] != before["updated_by"] || after["updated_at"] != before["updated_at"] {
		t.Fatal("avatar changed revision attribution")
	}
	page := c.call("GET", "/api/v1/articles/"+id+"/versions", nil, 200)
	if page["items"].([]any)[0].(map[string]any)["updated_by_avatar"] != mid {
		t.Fatal("version list has stale avatar")
	}
	c.call("DELETE", "/api/v1/account/avatar", nil, 200)
	if v := c.call("GET", "/api/v1/articles/"+id+"/versions/1", nil, 200)["updated_by_avatar"]; v != nil && v != "" {
		t.Fatal("cleared avatar retained in version response")
	}
}
