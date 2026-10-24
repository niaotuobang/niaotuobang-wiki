package main

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/niaotuobang/niaotuobang-wiki/internal/wiki"
)

func configuredInt(key string, fallback int) (int, error) {
	s := os.Getenv(key)
	if s == "" {
		return fallback, nil
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}
func loadAuthConfig() (*wiki.AuthConfig, error) {
	secretFile := os.Getenv("AUTH_SECRET_FILE")
	if secretFile == "" {
		return nil, fmt.Errorf("AUTH_SECRET_FILE and email verification are required for non-demo serve")
	}
	info, e := os.Stat(secretFile)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("AUTH_SECRET_FILE must be an owner-only regular file")
	}
	secret, e := os.ReadFile(secretFile)
	if e != nil || len(secret) != 32 {
		return nil, fmt.Errorf("AUTH_SECRET_FILE must contain exactly 32 random bytes")
	}
	if env("AUTH_REGISTRATION_MODE", "invite_only") != "invite_only" {
		return nil, fmt.Errorf("only invite_only registration is supported")
	}
	vals := map[string]int{}
	for k, v := range map[string]int{"AUTH_SESSION_DAYS": 30, "AUTH_REAUTH_SECONDS": 300, "AUTH_CODE_TTL_SECONDS": 300, "AUTH_CODE_RESEND_SECONDS": 60, "AUTH_CODE_MAX_ATTEMPTS": 5, "AUTH_SEND_PER_TARGET_HOUR": 5, "AUTH_SEND_PER_TARGET_DAY": 10, "AUTH_SEND_PER_IP_HOUR": 30, "AUTH_EMAIL_DAILY_LIMIT": 100, "SMTP_PORT": 465, "SMTP_CONNECT_TIMEOUT_SECONDS": 5, "SMTP_SEND_TIMEOUT_SECONDS": 10} {
		n, e := configuredInt(k, v)
		if e != nil {
			return nil, e
		}
		vals[k] = n
	}
	ttl := time.Duration(vals["AUTH_CODE_TTL_SECONDS"]) * time.Second
	resend := time.Duration(vals["AUTH_CODE_RESEND_SECONDS"]) * time.Second
	// Templates and email copy currently use the agreed five-minute validity.
	if ttl != 5*time.Minute || resend < time.Minute {
		return nil, fmt.Errorf("verification TTL must be 300 seconds and resend interval at least 60 seconds")
	}

	email, e := wiki.NewSMTPSender(wiki.SMTPConfig{SiteURL: os.Getenv("BASE_URL"), Host: os.Getenv("SMTP_HOST"), ProxyURL: os.Getenv("SMTP_PROXY_URL"), Port: vals["SMTP_PORT"], TLSMode: env("SMTP_TLS_MODE", "implicit_tls"), User: os.Getenv("SMTP_USER"), Password: os.Getenv("SMTP_PASSWORD"), From: os.Getenv("SMTP_FROM"), ConnectTimeout: time.Duration(vals["SMTP_CONNECT_TIMEOUT_SECONDS"]) * time.Second, SendTimeout: time.Duration(vals["SMTP_SEND_TIMEOUT_SECONDS"]) * time.Second})
	if e != nil {
		return nil, e
	}
	var trusted []netip.Prefix
	for _, v := range strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",") {
		if strings.TrimSpace(v) == "" {
			continue
		}
		p, e := netip.ParsePrefix(strings.TrimSpace(v))
		if e != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS contains an invalid network")
		}
		trusted = append(trusted, p)
	}
	return &wiki.AuthConfig{InitialAdminEmail: os.Getenv("AUTH_INITIAL_ADMIN_EMAIL"), InitialAdminName: os.Getenv("AUTH_INITIAL_ADMIN_NAME"), AllowedRegistrationEmails: strings.FieldsFunc(os.Getenv("AUTH_REGISTRATION_EMAIL_ALLOWLIST"), func(r rune) bool { return r == ',' || r == '\n' }), TrustedProxies: trusted, SessionDays: vals["AUTH_SESSION_DAYS"], ReauthSeconds: vals["AUTH_REAUTH_SECONDS"], Secret: secret, Email: email, TTL: ttl, Resend: resend, MaxAttempts: vals["AUTH_CODE_MAX_ATTEMPTS"], TargetHour: vals["AUTH_SEND_PER_TARGET_HOUR"], TargetDay: vals["AUTH_SEND_PER_TARGET_DAY"], IPHour: vals["AUTH_SEND_PER_IP_HOUR"], EmailDay: vals["AUTH_EMAIL_DAILY_LIMIT"]}, nil
}
