package wiki

import (
	"crypto/rand"
	"testing"
)

func TestEmailOnlySetupAndAdministratorReauth(t *testing.T) {
	a, e := Open(Config{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	key := make([]byte, 32)
	rand.Read(key)
	s := &captureSender{}
	if e = a.initAuth(AuthConfig{Secret: key, Email: s}); e != nil {
		t.Fatal(e)
	}
	c := &testClient{t: t, app: a}
	c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "owner@example.com", "setup_token": "bad"}, 403)
	token := a.setupToken
	v := c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "owner@example.com", "setup_token": token}, 202)
	c.call("POST", "/api/v1/auth/setup", map[string]string{"token": token, "name": "Owner", "challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	options := c.call("GET", "/api/v1/auth/options", nil, 200)
	if options["password"] != false || options["email"] != true {
		t.Fatal("password authentication remains advertised")
	}
	var size int
	a.store.DB.QueryRow(`SELECT length(password_hash) FROM users`).Scan(&size)
	if size != 0 {
		t.Fatal("password generated during setup")
	}
	var recent string
	a.store.DB.QueryRow(`SELECT reauthenticated_at FROM sessions LIMIT 1`).Scan(&recent)
	if recent == "" {
		t.Fatal("verified email login did not authorize recent administrative actions")
	}
	c.call("POST", "/api/v1/auth/login", map[string]string{"login": "Owner", "password": "legacy-password"}, 410)
	c.call("POST", "/api/v1/account/reauth", map[string]string{"password": "legacy-password"}, 410)
	c.call("PUT", "/api/v1/account/password", map[string]string{"password": "new-password"}, 410)
	a.store.DB.Exec(`UPDATE auth_challenges SET created_at='2000-01-01T00:00:00Z'`)
	v = c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "reauth"}, 202)
	if s.last().target != "owner@example.com" {
		t.Fatal("reauth did not resolve bound mailbox")
	}
	c.call("POST", "/api/v1/account/reauth", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	v = c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "replace", "target": "new-owner@example.com"}, 202)
	c.call("POST", "/api/v1/account/identities/replace", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	c.call("GET", "/api/v1/articles", nil, 401)
}

func TestDisableAndEnableRequiresFreshEmailLogin(t *testing.T) {
	a, admin, s := identityTestApp(t)
	member := newUserClient(t, a, "editor")
	id := member.call("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"].(string)
	target := "member@example.com"
	v := member.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "bind", "target": target}, 202)
	member.call("POST", "/api/v1/account/identities/bind", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	advance := func() { a.store.DB.Exec(`UPDATE auth_challenges SET created_at='2000-01-01T00:00:00Z'`) }
	advance()
	live := &testClient{t: t, app: a}
	v = live.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": target}, 202)
	live.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	advance()
	pending := &testClient{t: t, app: a}
	proof := pending.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": target}, 202)
	oldCode := s.last().code
	admin.call("PATCH", "/api/v1/admin/members/"+id, map[string]any{"role": "user", "can_edit": true, "disabled": true}, 200)
	live.call("GET", "/api/v1/articles", nil, 401)
	live.call("GET", "/media/private-image", nil, 401)
	live.call("HEAD", "/media/private-video", nil, 401)
	pending.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": proof["challenge_id"].(string), "code": oldCode}, 400)
	advance()
	a.auth.registrationEmails[a.auth.digest("identity-index", "email", target)] = true
	before := len(s.messages)
	(&testClient{t: t, app: a}).call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": target}, 202)
	if len(s.messages) != before {
		t.Fatal("disabled allowlisted account received a login code")
	}
	admin.call("PATCH", "/api/v1/admin/members/"+id, map[string]any{"role": "user", "can_edit": true, "disabled": false}, 200)
	live.call("GET", "/api/v1/articles", nil, 401)
	advance()
	fresh := &testClient{t: t, app: a}
	v = fresh.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": target}, 202)
	fresh.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	fresh.call("GET", "/api/v1/articles", nil, 200)
}
