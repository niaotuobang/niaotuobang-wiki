package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type failingCodeSender struct{ calls int }

func (s *failingCodeSender) Send(context.Context, string, string, string, string) error {
	s.calls++
	return &DeliveryError{Code: "network_or_timeout"}
}

func TestSMSQuotaPersistsAfterFailureAndReload(t *testing.T) {
	for _, window := range []string{"day", "month"} {
		t.Run(window, func(t *testing.T) {
			a, admin, _ := identityTestApp(t)
			if a.auth.cfg.SMSDay != 20 || a.auth.cfg.SMSMonth != 100 {
				t.Fatal("unexpected default SMS budget")
			}
			sender := &failingCodeSender{}
			cfg := a.auth.cfg
			cfg.Phone = sender
			if window == "day" {
				cfg.SMSDay = 1
			} else {
				cfg.SMSMonth = 1
			}
			a.auth.cfg = cfg
			invite := admin.call("POST", "/api/v1/admin/invites", map[string]string{"role": "user"}, 201)["invite"].(string)
			guest := &testClient{t: t, app: a}
			guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000000", "invite": invite}, 202)
			// A new application instance must load the same persistent budget,
			// including attempts whose delivery could not be confirmed.
			reloaded, err := Open(a.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer reloaded.Close()
			if err := reloaded.initAuth(cfg); err != nil {
				t.Fatal(err)
			}
			guest.app = reloaded
			guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000001", "invite": invite}, 429)
			if sender.calls != 1 {
				t.Fatal("quota allowed another provider call")
			}
		})
	}
}

func TestConsumedInviteAndCodeOnlyJoinOnceUnderConcurrency(t *testing.T) {
	a, admin, s := identityTestApp(t)
	inv := admin.call("POST", "/api/v1/admin/invites", map[string]string{"role": "user"}, 201)["invite"].(string)
	guest := &testClient{t: t, app: a}
	v := guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "join", "target": "concurrent@example.com", "invite": inv}, 202)
	body, _ := json.Marshal(map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code, "invite": inv, "nickname": "测试"})
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/v1/auth/join", strings.NewReader(string(body)))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(guest.cookie)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			statuses <- w.Code
		}()
	}
	wg.Wait()
	close(statuses)
	success := 0
	for status := range statuses {
		if status == 200 {
			success++
		} else if status != 400 && status != 403 {
			t.Fatalf("unexpected status %d", status)
		}
	}
	if success != 1 {
		t.Fatal("invite/code consumed more than once")
	}
}
func TestOldIdentityCodeRevokedWhenContactChanges(t *testing.T) {
	a, c, s := identityTestApp(t)
	v := c.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "bind", "target": "old-contact@example.com"}, 202)
	c.call("POST", "/api/v1/account/identities/bind", map[string]string{"challenge_id": v["challenge_id"].(string), "code": s.last().code}, 200)
	a.store.DB.Exec(`UPDATE auth_challenges SET created_at=?`, time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339Nano))
	anon := &testClient{t: t, app: a}
	p := anon.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "login", "target": "old-contact@example.com"}, 202)
	code := s.last().code
	// Model an administrator-authorized binding replacement before this code is redeemed.
	a.store.DB.Exec(`DELETE FROM user_identities WHERE kind='email'`)
	anon.call("POST", "/api/v1/auth/login-code", map[string]string{"challenge_id": p["challenge_id"].(string), "code": code}, 400)
}
func TestVerificationEmailAndSenderValidation(t *testing.T) {
	raw, e := mailCodeMessage("sender@example.com", "member@example.com", "123456", "bind")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), "multipart/alternative") || !strings.Contains(string(raw), "123456") {
		t.Fatal("verification mail omitted alternatives or code")
	}
	if _, e = mailCodeMessage("sender@example.com", "member@example.com\r\nBcc: other@example.com", "123456", "bind"); e == nil {
		t.Fatal("mail header injection accepted")
	}
	if _, e = NewSMTPSender(SMTPConfig{Host: "smtp.example.com", User: "sender@example.com", From: "another@example.com", Password: "test-only"}); e == nil {
		t.Fatal("different SMTP sender identity accepted")
	}
}
func TestAnonymousCannotReadExistingMediaWithLegacySetting(t *testing.T) {
	a, c := newTestApp(t)
	a.store.DB.Exec(`INSERT INTO settings(key,value) VALUES('public_read','true')`)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := c.raw(method, "/media/missing", nil, true)
		if w.Code != 404 {
			t.Fatal("authenticated media lookup changed")
		}
		anon := &testClient{t: t, app: a}
		w = anon.raw(method, "/media/missing", nil, false)
		if w.Code != 401 {
			t.Fatal("anonymous media existence revealed")
		}
	}
}

func TestRevokedInviteCannotSendJoinCode(t *testing.T) {
	a, c, _ := identityTestApp(t)
	token := c.call("POST", "/api/v1/admin/invites", map[string]string{"role": "user"}, 201)["invite"].(string)
	c.call("DELETE", "/api/v1/admin/invites/"+hashToken(token), nil, 200)
	guest := &testClient{t: t, app: a}
	guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "email", "purpose": "join", "target": "revoked@example.com", "invite": token}, 403)
	list := c.call("GET", "/api/v1/admin/invites", nil, 200)
	if list["items"].([]any)[0].(map[string]any)["state"] != "revoked" {
		t.Fatal("invite history did not retain revocation")
	}
}
func TestMonthlySMSLimitAndProxyTrust(t *testing.T) {
	a, c, _ := identityTestApp(t)
	a.auth.cfg.SMSMonth = 1
	inv := c.call("POST", "/api/v1/admin/invites", map[string]string{"role": "user"}, 201)["invite"].(string)
	guest := &testClient{t: t, app: a}
	guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000000", "invite": inv}, 202)
	guest.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "join", "target": "13800000001", "invite": inv}, 429)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.1")
	if a.clientIP(r) != "192.0.2.1" {
		t.Fatal("untrusted proxy header accepted")
	}
}

func TestDisabledPhoneRejectsSendingAndOutstandingCodes(t *testing.T) {
	a, admin, sender := identityTestApp(t)
	proof := admin.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "bind", "target": "13800000000"}, 202)
	code := sender.last().code
	a.auth.cfg.Phone = nil
	options := admin.call("GET", "/api/v1/auth/options", nil, 200)
	if options["phone"] != false || options["email"] != true {
		t.Fatal("wrong available channels")
	}
	admin.call("POST", "/api/v1/account/identities/bind", map[string]string{"challenge_id": proof["challenge_id"].(string), "code": code}, 400)
	admin.call("POST", "/api/v1/auth/challenges", map[string]string{"channel": "phone", "purpose": "bind", "target": "13800000001"}, 503)
	if len(sender.messages) != 1 {
		t.Fatal("disabled channel invoked sender")
	}
}
