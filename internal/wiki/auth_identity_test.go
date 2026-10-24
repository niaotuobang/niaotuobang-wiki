package wiki

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type capturedCode struct{ target, code, purpose, id string }
type captureSender struct {
	mu       sync.Mutex
	messages []capturedCode
}

func (s *captureSender) Send(_ context.Context, target, code, purpose, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, capturedCode{target, code, purpose, id})
	return nil
}
func (s *captureSender) last() capturedCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[len(s.messages)-1]
}
func identityTestApp(t *testing.T) (*App, *testClient, *captureSender) {
	t.Helper()
	a, c := newTestApp(t)
	s := a.auth.cfg.Email.(*captureSender)
	return a, c, s
}
func TestIdentityBindingPurposeSessionAndEncryption(t *testing.T) {
	a, c, s := identityTestApp(t)
	proof := c.call("POST", "/api/v1/auth/challenges", map[string]any{"channel": "email", "purpose": "bind", "target": "Test@example.com"}, 202)
	code := s.last().code
	id := proof["challenge_id"].(string)
	other := newUserClient(t, a, "editor")
	other.call("POST", "/api/v1/account/identities/bind", map[string]string{"challenge_id": id, "code": code}, 400)
	c.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": id, "code": code}, 400)
	c.call("POST", "/api/v1/account/identities/bind", map[string]string{"challenge_id": id, "code": code}, 200)
	c.call("GET", "/api/v1/articles", nil, 401)
	var cipher []byte
	if e := a.store.DB.QueryRow(`SELECT address_cipher FROM user_identities LIMIT 1`).Scan(&cipher); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(cipher), "example.com") {
		t.Fatal("contact stored as plaintext")
	}
	_, e := a.store.DB.Exec(`UPDATE auth_challenges SET created_at=?`, time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339Nano))
	if e != nil {
		t.Fatal(e)
	}
	anon := &testClient{t: t, app: a}
	login := anon.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": "test@example.com"}, 202)
	anon.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": login["challenge_id"].(string), "code": s.last().code}, 200)
	anon.call("GET", "/api/v1/articles", nil, 200)
	identity := anon.call("GET", "/api/v1/account/identities", nil, 200)
	b, _ := json.Marshal(identity)
	if strings.Contains(string(b), "test@example.com") {
		t.Fatal("full email exposed in account response")
	}
}
func TestCodeAttemptsInviteAndRateLimits(t *testing.T) {
	a, c, s := identityTestApp(t)
	invite := c.call("POST", "/api/v1/admin/invites", map[string]string{"role": "user"}, 201)["invite"].(string)
	guest := &testClient{t: t, app: a}
	guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000000", "invite": "invalid"}, 403)
	p := guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000000", "invite": invite}, 202)
	guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000000", "invite": invite}, 429)
	id := p["challenge_id"].(string)
	code := s.last().code
	for range 5 {
		guest.call("POST", "/api/v1/auth/join", map[string]string{"challenge_id": id, "code": "bad", "invite": invite, "nickname": "测试成员"}, 400)
	}
	guest.call("POST", "/api/v1/auth/join", map[string]string{"challenge_id": id, "code": code, "invite": invite, "nickname": "测试成员"}, 400)
	var attempts int
	a.store.DB.QueryRow(`SELECT attempts FROM auth_challenges WHERE id=?`, id).Scan(&attempts)
	if attempts != 5 {
		t.Fatal("wrong attempts were not persisted")
	}
	a.store.DB.Exec(`UPDATE auth_challenges SET created_at=?`, time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339Nano))
	p = guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000000", "invite": invite}, 202)
	guest.call("POST", "/api/v1/auth/join", map[string]string{"challenge_id": p["challenge_id"].(string), "code": s.last().code, "invite": invite, "nickname": "测试成员"}, 200)
	guest.call("GET", "/api/v1/articles", nil, 200)
	if s.last().target != "+8613800000000" {
		t.Fatal("phone normalization failed")
	}
	c.call("POST", "/api/v1/admin/settings", map[string]bool{"public_read": true}, 404)
}
func TestRolesReauthAndUnknownIdentity(t *testing.T) {
	a, system, s := identityTestApp(t)
	su := system.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)
	cipher, _ := a.auth.encrypt("email", "owner@example.com")
	a.store.DB.Exec(`INSERT INTO user_identities(id,user_id,kind,address_cipher,lookup_hash,verified_at) VALUES(?,?,?,?,?,?)`, newID(), su["id"], "email", cipher, a.auth.digest("identity-index", "email", "owner@example.com"), now())
	admin := newUserClient(t, a, "admin")
	member := newUserClient(t, a, "editor")
	uid := member.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"].(string)
	admin.call("PATCH", "/api/v1/admin/members/"+uid, map[string]any{"role": "system_admin", "disabled": false}, 403)
	admin.call("GET", "/api/v1/admin/backup", nil, 403)
	system.call("PATCH", "/api/v1/admin/members/"+uid, map[string]any{"role": "admin", "disabled": false}, 403)
	v := system.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "reauth"}, 202)
	system.call("POST", "/api/v1/account/reauth", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	system.call("PATCH", "/api/v1/admin/members/"+uid, map[string]any{"role": "admin", "disabled": false}, 200)
	member.call("GET", "/api/v1/articles", nil, 401)
	before := len(s.messages)
	unknown := &testClient{t: t, app: a}
	p := unknown.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": "nobody@example.com"}, 202)
	if len(s.messages) != before {
		t.Fatal("unknown identity triggered send")
	}
	unknown.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": p["challenge_id"].(string), "code": "000000"}, 400)
}
func TestSecretMismatchAndPrivateCache(t *testing.T) {
	a, c, _ := identityTestApp(t)
	bad := make([]byte, 32)
	rand.Read(bad)
	if e := a.initAuth(AuthConfig{Secret: bad}); e == nil {
		t.Fatal("wrong master key accepted")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.AddCookie(c.cookie)
	a.Handler().ServeHTTP(w, r)
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("private response cache allowed")
	}
}
