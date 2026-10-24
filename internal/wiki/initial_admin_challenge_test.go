package wiki

import (
	"crypto/rand"
	"testing"
)

func TestInitialAdministratorConfigRejectsEarlierMailboxChallenge(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	s := &captureSender{}
	cfg := Config{DataDir: t.TempDir(), Auth: &AuthConfig{Secret: key, Email: s, InitialAdminEmail: "first@example.test"}}
	a, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := &testClient{t: t, app: a}
	token := a.setupToken
	proof := c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "first@example.test", "setup_token": token}, 202)
	code := s.last().code
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.Auth.InitialAdminEmail = "second@example.test"
	a, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	c.app = a
	c.call("POST", "/api/v1/auth/setup", map[string]string{"token": token, "name": "Owner", "challenge_id": proof["challenge_id"].(string), "code": code}, 403)
	var count int
	if err := a.store.DB.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("previous mailbox initialized administrator after configuration changed")
	}
}
