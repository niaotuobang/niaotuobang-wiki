package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmailOnlyConfiguration(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(key, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"AUTH_SECRET_FILE": key, "SMTP_HOST": "smtp.example.com", "SMTP_USER": "sender@example.com", "SMTP_FROM": "sender@example.com", "SMTP_PASSWORD": "test-only-password", "SMS_PROVIDER": "unused", "SMS_ACCESS_KEY_ID": "", "SMS_ACCESS_KEY_SECRET": ""} {
		t.Setenv(k, v)
	}
	cfg, err := loadAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Phone != nil || cfg.Email == nil {
		t.Fatal("expected email-only authentication")
	}
	t.Setenv("SMTP_PASSWORD", "")
	if _, err := loadAuthConfig(); err == nil {
		t.Fatal("missing email configuration accepted")
	}
}
