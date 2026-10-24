package wiki

import (
	"crypto/rand"
	"strings"
	"testing"
)

func TestConfiguredInitialAdministratorRequiresMatchingVerifiedMailbox(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	s := &captureSender{}
	cfg := Config{DataDir: t.TempDir(), Auth: &AuthConfig{Secret: key, Email: s, InitialAdminEmail: "Owner@Example.test", InitialAdminName: "初始化管理员"}}
	a, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := &testClient{t: t, app: a}
	token := a.setupToken
	c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": "owner@example.test"}, 403)
	c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "other@example.test", "setup_token": token}, 403)
	options := c.call("GET", "/api/v1/auth/options", nil, 200)
	if options["setup_admin_configured"] != true || options["setup_name_configured"] != true {
		t.Fatal("configuration not advertised")
	}
	for _, v := range options {
		if text, ok := v.(string); ok && strings.Contains(text, "example.test") {
			t.Fatal("private initial mailbox exposed")
		}
	}
	v := c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "owner@example.test", "setup_token": token}, 202)
	result := c.call("POST", "/api/v1/auth/setup", map[string]string{"token": token, "name": "浏览器提供的名字", "challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	u := result["user"].(map[string]any)
	if u["name"] != "初始化管理员" || u["role"] != "system_admin" {
		t.Fatal("configured administrator not created")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.Auth.InitialAdminEmail = "different@example.test"
	cfg.Auth.InitialAdminName = "不会覆盖已有账号"
	a, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var count int
	var name string
	a.store.DB.QueryRow(`SELECT count(*) FROM users`).Scan(&count)
	a.store.DB.QueryRow(`SELECT name FROM users WHERE id=?`, u["id"]).Scan(&name)
	if count != 1 || name != "初始化管理员" || a.setupToken != "" {
		t.Fatal("restart changed existing administrator")
	}
	(&testClient{t: t, app: a}).call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "setup", "target": "different@example.test", "setup_token": token}, 403)
}

func TestInvalidInitialAdministratorConfiguration(t *testing.T) {
	for _, cfg := range []AuthConfig{{InitialAdminEmail: "invalid"}, {InitialAdminName: "name without email"}, {InitialAdminEmail: "owner@example.test", InitialAdminName: strings.Repeat("长", 41)}} {
		cfg.Secret = make([]byte, 32)
		a, err := Open(Config{DataDir: t.TempDir(), Auth: &cfg})
		if err == nil {
			a.Close()
			t.Fatal("invalid initial administrator accepted")
		}
	}
}
