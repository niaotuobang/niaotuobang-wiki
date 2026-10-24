package wiki

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// CodeSender delivers only verification messages; it must never log target or code.
type CodeSender interface {
	Send(context.Context, string, string, string, string) error
}
type AuthConfig struct {
	InitialAdminEmail, InitialAdminName                                    string
	AllowedRegistrationEmails                                              []string
	TrustedProxies                                                         []netip.Prefix
	SessionDays, ReauthSeconds                                             int
	Secret                                                                 []byte
	Phone, Email                                                           CodeSender
	TTL, Resend                                                            time.Duration
	MaxAttempts, TargetHour, TargetDay, IPHour, SMSDay, SMSMonth, EmailDay int
}
type authService struct {
	cfg                AuthConfig
	slots              chan struct{}
	registrationEmails map[string]bool
}

func (a *App) initAuth(c AuthConfig) error {
	c.InitialAdminEmail = strings.TrimSpace(c.InitialAdminEmail)
	c.InitialAdminName = strings.TrimSpace(c.InitialAdminName)
	if c.InitialAdminEmail != "" {
		address, err := normalizeIdentity("email", c.InitialAdminEmail)
		if err != nil {
			return fmt.Errorf("invalid initial administrator email")
		}
		c.InitialAdminEmail = address
	}
	if len([]rune(c.InitialAdminName)) > 40 || c.InitialAdminName != "" && c.InitialAdminEmail == "" {
		return fmt.Errorf("initial administrator name requires an email and must not exceed 40 characters")
	}
	if len(c.Secret) != 32 {
		return fmt.Errorf("auth secret must contain exactly 32 random bytes")
	}
	if c.SessionDays == 0 {
		c.SessionDays = 30
	}
	if c.ReauthSeconds == 0 {
		c.ReauthSeconds = 300
	}
	if c.SessionDays < 1 || c.SessionDays > 90 || c.ReauthSeconds < 1 || c.ReauthSeconds > 900 {
		return fmt.Errorf("invalid session or reauthentication lifetime")
	}
	if c.TTL == 0 {
		c.TTL = 5 * time.Minute
	}
	if c.Resend == 0 {
		c.Resend = time.Minute
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = 5
	}
	if c.TargetHour == 0 {
		c.TargetHour = 5
	}
	if c.TargetDay == 0 {
		c.TargetDay = 10
	}
	if c.IPHour == 0 {
		c.IPHour = 30
	}
	if c.SMSMonth == 0 {
		c.SMSMonth = 100
	}
	if c.SMSDay == 0 {
		c.SMSDay = 20
	}
	if c.EmailDay == 0 {
		c.EmailDay = 100
	}
	if c.TTL < time.Minute || c.TTL > 10*time.Minute || c.Resend < time.Minute || c.MaxAttempts < 1 || c.MaxAttempts > 5 || c.TargetHour < 1 || c.TargetDay < 1 || c.IPHour < 1 || c.SMSDay < 1 || c.SMSMonth < 1 || c.EmailDay < 1 {
		return fmt.Errorf("invalid authentication limits")
	}
	c.Secret = append([]byte(nil), c.Secret...)
	service := &authService{cfg: c, slots: make(chan struct{}, 2)}
	service.registrationEmails = map[string]bool{}
	if len(c.AllowedRegistrationEmails) > 100 {
		return fmt.Errorf("registration email allowlist is limited to 100 entries")
	}
	for _, address := range c.AllowedRegistrationEmails {
		address, err := normalizeIdentity("email", address)
		if err != nil {
			return fmt.Errorf("invalid registration email allowlist")
		}
		service.registrationEmails[service.digest("identity-index", "email", address)] = true
	}
	check := service.digest("key-check", "wiki-v1")
	var stored string
	e := a.store.DB.QueryRow(`SELECT value FROM settings WHERE key='auth_key_check'`).Scan(&stored)
	if e == sql.ErrNoRows {
		var count int
		a.store.DB.QueryRow(`SELECT count(*) FROM user_identities`).Scan(&count)
		if count > 0 {
			return fmt.Errorf("identity encryption key metadata is missing")
		}
		_, e = a.store.DB.Exec(`INSERT INTO settings(key,value) VALUES('auth_key_check',?)`, check)
	} else if e == nil && subtle.ConstantTimeCompare([]byte(stored), []byte(check)) != 1 {
		return fmt.Errorf("authentication secret does not match this database")
	}
	if e == nil {
		a.auth = service
	}
	return e
}
func (s *authService) key(label string) []byte {
	h := hmac.New(sha256.New, s.cfg.Secret)
	h.Write([]byte("wiki/auth/v1/" + label))
	return h.Sum(nil)
}
func (s *authService) digest(label string, values ...string) string {
	h := hmac.New(sha256.New, s.key(label))
	for _, v := range values {
		fmt.Fprintf(h, "%d:", len(v))
		h.Write([]byte(v))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *authService) encrypt(kind, value string) ([]byte, error) {
	b, e := aes.NewCipher(s.key("identity-encryption"))
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return g.Seal(nonce, nonce, []byte(value), []byte(kind)), nil
}
func (s *authService) decrypt(kind string, value []byte) (string, error) {
	b, e := aes.NewCipher(s.key("identity-encryption"))
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	if len(value) < g.NonceSize() {
		return "", fmt.Errorf("invalid encrypted identity")
	}
	out, e := g.Open(nil, value[:g.NonceSize()], value[g.NonceSize():], []byte(kind))
	return string(out), e
}

var domesticPhone = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

func normalizeIdentity(kind, target string) (string, error) {
	target = strings.TrimSpace(target)
	if strings.ContainsAny(target, "\r\n\x00") {
		return "", fail(400, "VALIDATION_ERROR", "联系方式格式不正确")
	}
	switch kind {
	case "phone":
		target = strings.TrimPrefix(target, "+86")
		if domesticPhone.MatchString(target) {
			return "+86" + target, nil
		}
	case "email":
		target = strings.ToLower(target)
		m, e := mail.ParseAddress(target)
		if e == nil && m.Address == target && len(target) <= 254 && strings.Contains(target, "@") && !strings.ContainsAny(target, " <>") {
			return target, nil
		}
	}
	return "", fail(400, "VALIDATION_ERROR", "请输入有效的国内手机号或邮箱")
}
func maskedIdentity(kind, target string) string {
	if kind == "phone" && len(target) >= 11 {
		return target[:6] + "****" + target[len(target)-4:]
	}
	parts := strings.SplitN(target, "@", 2)
	if len(parts) == 2 {
		return string([]rune(parts[0])[:1]) + "***@" + parts[1]
	}
	return "***"
}
func (a *App) authReady() error {
	if a.auth == nil {
		return fail(503, "AUTH_NOT_CONFIGURED", "邮箱验证码服务尚未配置，请联系管理员")
	}
	return nil
}
func (a *App) authSubject(r *http.Request) string {
	if _, t := a.session(r); t != "" {
		return "session:" + hashToken(t)
	}
	if c, e := r.Cookie("wiki_pre_auth"); e == nil && len(c.Value) == 64 {
		if _, e = hex.DecodeString(c.Value); e == nil {
			return "pre:" + hashToken(c.Value)
		}
	}
	return ""
}
func (a *App) ensureAuthSubject(w http.ResponseWriter, r *http.Request) string {
	if s := a.authSubject(r); s != "" {
		return s
	}
	t := newID() + newID()
	http.SetCookie(w, &http.Cookie{Name: "wiki_pre_auth", Value: t, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.BaseURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 900})
	return "pre:" + hashToken(t)
}
func reserveRate(tx *sql.Tx, key string, window time.Duration, limit int) error {
	n := time.Now().UTC()
	slot := n.Unix() / int64(window/time.Second)
	k := fmt.Sprintf("%s:%d", key, slot)
	return reserveCounter(tx, k, n.Add(window+24*time.Hour), limit)
}
func reserveCounter(tx *sql.Tx, key string, expires time.Time, limit int) error {
	var count int
	e := tx.QueryRow(`INSERT INTO auth_rate_limits(key,count,expires_at) VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET count=count+1 RETURNING count`, key, expires.Format(time.RFC3339Nano)).Scan(&count)
	if e != nil {
		return e
	}
	if count > limit {
		return fail(429, "RATE_LIMITED", "操作过于频繁，请稍后重试")
	}
	return nil
}
func audit(tx *sql.Tx, actor, event, object string) error {
	_, e := tx.Exec(`INSERT INTO auth_audit_events(id,actor_id,event,object_id,result,created_at) VALUES(?,?,?,?,?,?)`, newID(), actor, event, object, "success", now())
	return e
}
func (a *App) requireReauth(r *http.Request, u *User) error {
	if u == nil {
		return fail(401, "UNAUTHENTICATED", "请先登录")
	}
	_, t := a.session(r)
	var date string
	if e := a.store.DB.QueryRow(`SELECT reauthenticated_at FROM sessions WHERE token_hash=?`, hashToken(t)).Scan(&date); e != nil {
		return e
	}
	at, e := time.Parse(time.RFC3339Nano, date)
	if e != nil || time.Since(at) > a.reauthWindow() {
		return fail(403, "REAUTH_REQUIRED", "请先重新验证本人身份")
	}
	return nil
}

type codeProof struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}
type challenge struct {
	ID, Kind, Purpose, Lookup, Subject, UserID, InviteHash, CodeHash, Expires, State string
	Cipher                                                                           []byte
	Attempts                                                                         int
}

func (a *App) withChallenge(r *http.Request, p codeProof, purpose string, fn func(*sql.Tx, challenge) error) error {
	if e := a.authReady(); e != nil {
		return e
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	subject := a.authSubject(r)
	tx, e := a.store.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if subject == "" {
		return fail(400, "INVALID_CODE", "验证码无效或已过期")
	}
	if e = reserveRate(tx, "verify:"+hashToken(subject), time.Minute, 30); e != nil {
		return e
	}
	var c challenge
	e = tx.QueryRow(`SELECT id,kind,purpose,lookup_hash,address_cipher,subject,user_id,invite_hash,code_hash,expires_at,attempts,state FROM auth_challenges WHERE id=?`, p.ChallengeID).Scan(&c.ID, &c.Kind, &c.Purpose, &c.Lookup, &c.Cipher, &c.Subject, &c.UserID, &c.InviteHash, &c.CodeHash, &c.Expires, &c.Attempts, &c.State)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	channelEnabled := c.Kind == "email" && a.auth.cfg.Email != nil || c.Kind == "phone" && a.auth.cfg.Phone != nil
	valid := e == nil && channelEnabled && c.Subject == subject && c.Purpose == purpose && c.State == "sent" && c.Expires > now() && c.Attempts < a.auth.cfg.MaxAttempts
	expected := a.auth.digest("verification", c.ID, c.Kind, c.Lookup, c.Purpose, c.Subject, p.Code)
	if !valid || subtle.ConstantTimeCompare([]byte(expected), []byte(c.CodeHash)) != 1 {
		if valid {
			if _, e = tx.Exec(`UPDATE auth_challenges SET attempts=attempts+1 WHERE id=?`, c.ID); e != nil {
				return e
			}
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		return fail(400, "INVALID_CODE", "验证码无效或已过期")
	}
	if _, e = tx.Exec(`UPDATE auth_challenges SET state='consumed' WHERE id=?`, c.ID); e != nil {
		return e
	}
	if e = fn(tx, c); e != nil {
		return e
	}
	return tx.Commit()
}
func userByID(q querier, id string) (User, error) {
	var u User
	e := q.QueryRow(`SELECT id,name,login,access_role,can_edit,coalesce(avatar_media_id,'') FROM users WHERE id=? AND disabled=0`, id).Scan(&u.ID, &u.Name, &u.Login, &u.Role, &u.CanEdit, &u.AvatarMediaID)
	if e == sql.ErrNoRows {
		return u, fail(400, "INVALID_CODE", "验证码无效或已过期")
	}
	return u, e
}
func (a *App) makeSession(db execer, u User) (string, error) {
	token := newID() + newID()
	_, e := db.Exec(`INSERT INTO sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)`, hashToken(token), u.ID, time.Now().UTC().Add(a.sessionAge()).Format(time.RFC3339Nano), now())
	return token, e
}
func (a *App) makeVerifiedSession(db execer, u User) (string, error) {
	token, e := a.makeSession(db, u)
	if e != nil {
		return "", e
	}
	_, e = db.Exec(`UPDATE sessions SET reauthenticated_at=? WHERE token_hash=?`, now(), hashToken(token))
	return token, e
}
func (a *App) sessionResponse(w http.ResponseWriter, u User, token string) {
	http.SetCookie(w, &http.Cookie{Name: "wiki_session", Value: token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.BaseURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: int(a.sessionAge().Seconds())})
	writeJSON(w, 200, map[string]any{"user": u, "csrf_token": csrfToken(token)})
}
func (a *App) issueChallenge(w http.ResponseWriter, r *http.Request, u *User) error {
	if e := a.authReady(); e != nil {
		return e
	}
	var b struct {
		Channel    string `json:"channel"`
		Purpose    string `json:"purpose"`
		Target     string `json:"target"`
		Invite     string `json:"invite"`
		SetupToken string `json:"setup_token"`
	}
	if e := decode(w, r, &b); e != nil {
		return e
	}
	if b.Purpose == "reauth" && b.Channel == "email" && b.Target == "" && u != nil {
		var encrypted []byte
		if err := a.store.DB.QueryRow(`SELECT address_cipher FROM user_identities WHERE user_id=? AND kind='email'`, u.ID).Scan(&encrypted); err != nil {
			return fail(400, "EMAIL_REQUIRED", "请先绑定邮箱")
		}
		var err error
		b.Target, err = a.auth.decrypt("email", encrypted)
		if err != nil {
			return err
		}
	}
	target, e := normalizeIdentity(b.Channel, b.Target)
	if e != nil {
		return e
	}
	sender := a.auth.cfg.Email
	if b.Channel == "phone" {
		sender = a.auth.cfg.Phone
	}
	if sender == nil {
		return fail(503, "CHANNEL_UNAVAILABLE", "此验证渠道暂不可用，请使用其他登录方式")
	}
	switch b.Purpose {
	case "login", "join", "recover", "setup":
	case "bind", "replace", "reauth":
		if u == nil {
			return fail(401, "UNAUTHENTICATED", "请先登录")
		}
	default:
		return fail(400, "VALIDATION_ERROR", "验证用途不正确")
	}
	if b.Purpose == "replace" {
		if e = a.requireReauth(r, u); e != nil {
			return e
		}
	}
	select {
	case a.auth.slots <- struct{}{}:
		defer func() { <-a.auth.slots }()
	default:
		return fail(429, "RATE_LIMITED", "请求繁忙，请稍后重试")
	}
	subject := a.ensureAuthSubject(w, r)
	lookup := a.auth.digest("identity-index", b.Channel, target)
	id := newID()
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return e
	}
	code := fmt.Sprintf("%06d", n.Int64())
	encrypted, e := a.auth.encrypt(b.Channel, target)
	if e != nil {
		return e
	}
	host := a.clientIP(r)
	a.authMu.Lock()
	tx, e := a.store.DB.Begin()
	if e != nil {
		a.authMu.Unlock()
		return e
	}
	finish := func(err error) error { tx.Rollback(); a.authMu.Unlock(); return err }
	if a.setupToken != "" && b.Purpose != "setup" {
		return finish(fail(403, "SETUP_REQUIRED", "请先完成管理员初始化"))
	}
	uid := ""
	send := true
	if b.Purpose == "setup" {
		if b.Channel != "email" || a.setupToken == "" || subtle.ConstantTimeCompare([]byte(a.setupToken), []byte(b.SetupToken)) != 1 {
			return finish(fail(403, "FORBIDDEN", "初始化凭证无效"))
		}
		if a.auth.cfg.InitialAdminEmail != "" && target != a.auth.cfg.InitialAdminEmail {
			return finish(fail(403, "FORBIDDEN", "请使用服务器配置的初始化管理员邮箱"))
		}
		b.Invite = b.SetupToken
	}
	if b.Purpose == "join" {
		var role string
		if e = tx.QueryRow(`SELECT role FROM invites WHERE token_hash=? AND expires_at>? AND revoked_at='' AND consumed_at=''`, hashToken(b.Invite), now()).Scan(&role); e != nil {
			return finish(fail(403, "INVALID_INVITE", "邀请无效或已过期"))
		}
	}
	if b.Purpose == "login" || b.Purpose == "recover" || b.Purpose == "reauth" {
		var role string
		e = tx.QueryRow(`SELECT u.id,u.access_role FROM user_identities i JOIN users u ON u.id=i.user_id WHERE i.kind=? AND i.lookup_hash=? AND u.disabled=0`, b.Channel, lookup).Scan(&uid, &role)
		if e != nil && e != sql.ErrNoRows {
			return finish(e)
		}
		if e == sql.ErrNoRows || b.Purpose == "reauth" && (u == nil || uid != u.ID) {
			send = false
		}
		if e == sql.ErrNoRows && b.Purpose == "login" && b.Channel == "email" && a.auth.registrationEmails[lookup] {
			var existing int
			if e = tx.QueryRow(`SELECT count(*) FROM user_identities WHERE kind='email' AND lookup_hash=?`, lookup).Scan(&existing); e != nil {
				return finish(e)
			}
			send = existing == 0
		}
	} else if u != nil {
		uid = u.ID
	}
	var latest string
	e = tx.QueryRow(`SELECT created_at FROM auth_challenges WHERE lookup_hash=? ORDER BY created_at DESC LIMIT 1`, lookup).Scan(&latest)
	if e != nil && e != sql.ErrNoRows {
		return finish(e)
	}
	if at, err := time.Parse(time.RFC3339Nano, latest); err == nil && time.Since(at) < a.auth.cfg.Resend {
		return finish(fail(429, "RATE_LIMITED", "请等待一分钟后再发送"))
	}
	limits := []struct {
		key    string
		window time.Duration
		limit  int
	}{{"target-hour:" + lookup, time.Hour, a.auth.cfg.TargetHour}, {"target-day:" + lookup, 24 * time.Hour, a.auth.cfg.TargetDay}, {"ip:" + hashToken(host), time.Hour, a.auth.cfg.IPHour}}
	daily := a.auth.cfg.EmailDay
	if b.Channel == "phone" {
		daily = a.auth.cfg.SMSDay
	}
	limits = append(limits, struct {
		key    string
		window time.Duration
		limit  int
	}{"site:" + b.Channel, 24 * time.Hour, daily})
	if b.Channel == "phone" {
		n := time.Now().UTC()
		nextMonth := time.Date(n.Year(), n.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		if e = reserveCounter(tx, "sms-month:"+n.Format("2006-01"), nextMonth.Add(24*time.Hour), a.auth.cfg.SMSMonth); e != nil {
			return finish(e)
		}
	}
	for _, l := range limits {
		if e = reserveRate(tx, l.key, l.window, l.limit); e != nil {
			return finish(e)
		}
	}
	if _, e = tx.Exec(`UPDATE auth_challenges SET state='superseded' WHERE lookup_hash=? AND state IN ('pending','sent','decoy')`, lookup); e != nil {
		return finish(e)
	}
	state := "pending"
	if !send {
		state = "decoy"
	}
	_, e = tx.Exec(`INSERT INTO auth_challenges(id,kind,purpose,lookup_hash,address_cipher,subject,user_id,invite_hash,code_hash,expires_at,state,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, b.Channel, b.Purpose, lookup, encrypted, subject, uid, hashToken(b.Invite), a.auth.digest("verification", id, b.Channel, lookup, b.Purpose, subject, code), time.Now().UTC().Add(a.auth.cfg.TTL).Format(time.RFC3339Nano), state, now())
	if e != nil {
		return finish(e)
	}
	e = tx.Commit()
	a.authMu.Unlock()
	if e != nil {
		return e
	}
	if send {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		e = sender.Send(ctx, target, code, b.Purpose, id)
		result := deliveryResult(e)
		if _, err := a.store.DB.Exec(`INSERT INTO auth_audit_events(id,actor_id,event,object_id,result,created_at) VALUES(?,?,?,?,?,?)`, newID(), uid, "delivery."+b.Channel, id, result, now()); err != nil {
			return err
		}
		state = "sent"
		if e != nil {
			state = "unknown"
		}
		_, dbErr := a.store.DB.Exec(`UPDATE auth_challenges SET state=? WHERE id=? AND state='pending'`, state, id)
		if dbErr != nil {
			return dbErr
		}
		// Do not expose per-recipient provider failures or account existence.
		// The generic response promises neither delivery nor membership.

	}
	writeJSON(w, 202, map[string]any{"challenge_id": id, "resend_after": int(a.auth.cfg.Resend.Seconds()), "expires_in": int(a.auth.cfg.TTL.Seconds()), "message": "如该账号符合条件，将收到验证码"})
	return nil
}
func (a *App) identityRoutes() {
	a.avatarRoutes()
	a.route("GET /api/v1/auth/options", "public", func(w http.ResponseWriter, r *http.Request, u *User) error {
		phone, email := false, false
		setupEmail, setupName := false, false
		a.authMu.Lock()
		if a.auth != nil {
			phone = a.auth.cfg.Phone != nil
			email = a.auth.cfg.Email != nil
			setupEmail = a.setupToken != "" && a.auth.cfg.InitialAdminEmail != ""
			setupName = setupEmail && a.auth.cfg.InitialAdminName != ""
		}
		a.authMu.Unlock()
		writeJSON(w, 200, map[string]any{"phone": phone, "email": email, "password": false, "registration": "invite_only", "setup_admin_configured": setupEmail, "setup_name_configured": setupName})
		return nil
	})
	a.route("POST /api/v1/auth/challenges", "public", a.issueChallenge)
	for _, purpose := range []string{"login", "recover"} {
		path := "/api/v1/auth/login-code"
		if purpose == "recover" {
			path = "/api/v1/auth/recover"
		}
		a.route("POST "+path, "public", func(w http.ResponseWriter, r *http.Request, _ *User) error {
			var p codeProof
			if e := decode(w, r, &p); e != nil {
				return e
			}
			var u User
			var token string
			e := a.withChallenge(r, p, purpose, func(tx *sql.Tx, c challenge) error {
				var e error
				if purpose == "login" && c.UserID == "" && c.Kind == "email" && a.auth.registrationEmails[c.Lookup] {
					var existing int
					if e = tx.QueryRow(`SELECT count(*) FROM user_identities WHERE kind='email' AND lookup_hash=?`, c.Lookup).Scan(&existing); e != nil {
						return e
					}
					if existing != 0 {
						return fail(400, "INVALID_CODE", "验证码无效或已过期，请重新获取")
					}
					u = User{ID: newID(), Name: "群友", Login: "member-" + newID(), Role: "user", CanEdit: true}
					if e = insertUser(tx, u, []byte{}); e != nil {
						return e
					}
					if e = insertIdentity(tx, u.ID, c); e != nil {
						return e
					}
					if e = audit(tx, u.ID, "auth.allowlist_join", u.ID); e != nil {
						return e
					}
					c.UserID = u.ID
				}
				u, e = userByID(tx, c.UserID)
				if e != nil {
					return e
				}
				var identities int
				if e = tx.QueryRow(`SELECT count(*) FROM user_identities WHERE user_id=? AND kind=? AND lookup_hash=?`, c.UserID, c.Kind, c.Lookup).Scan(&identities); e != nil {
					return e
				}
				if identities != 1 {
					return fail(400, "INVALID_CODE", "验证码无效或已过期")
				}
				if _, e = tx.Exec(`UPDATE user_identities SET verified_at=? WHERE user_id=? AND kind=? AND lookup_hash=? AND verified_at=''`, now(), c.UserID, c.Kind, c.Lookup); e != nil {
					return e
				}

				if purpose == "recover" {
					if u.Role == "system_admin" {
						return fail(403, "FORBIDDEN", "系统管理员需使用服务器恢复流程")
					}
					if e = revokeCredentials(tx, u.ID); e != nil {
						return e
					}
				}
				token, e = a.makeVerifiedSession(tx, u)
				if e != nil {
					return e
				}
				return audit(tx, u.ID, "auth."+purpose, u.ID)
			})
			if e != nil {
				return e
			}
			a.sessionResponse(w, u, token)
			return nil
		})
	}
	a.route("POST /api/v1/auth/join", "public", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		var b struct {
			codeProof
			Invite   string `json:"invite"`
			Nickname string `json:"nickname"`
		}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		name := strings.TrimSpace(b.Nickname)
		if name == "" || len([]rune(name)) > 40 {
			return fail(400, "VALIDATION_ERROR", "请填写昵称")
		}
		var u User
		var token string
		e := a.withChallenge(r, b.codeProof, "join", func(tx *sql.Tx, c challenge) error {
			if c.InviteHash != hashToken(b.Invite) {
				return fail(403, "INVALID_INVITE", "邀请无效")
			}
			var role, recoveryUser string
			if e := tx.QueryRow(`SELECT role,recovery_user_id FROM invites WHERE token_hash=? AND expires_at>? AND revoked_at='' AND consumed_at=''`, c.InviteHash, now()).Scan(&role, &recoveryUser); e != nil {
				return fail(403, "INVALID_INVITE", "邀请无效或已使用")
			}
			if recoveryUser != "" {
				var e error
				u, e = userByID(tx, recoveryUser)
				if e != nil || u.Role != "system_admin" {
					return fail(403, "FORBIDDEN", "恢复链接不可用")
				}
				if _, e = tx.Exec(`DELETE FROM sessions WHERE user_id=?`, u.ID); e != nil {
					return e
				}
				if _, e = tx.Exec(`DELETE FROM user_identities WHERE user_id=? AND kind='email'`, u.ID); e != nil {
					return e
				}
				if e = audit(tx, u.ID, "system.email_recovery", u.ID); e != nil {
					return e
				}
			} else {
				u = User{ID: newID(), Name: name, Login: "member-" + newID(), Role: "user", CanEdit: role != "viewer"}
				if e := insertUser(tx, u, []byte{}); e != nil {
					return e
				}
			}
			if e := insertIdentity(tx, u.ID, c); e != nil {
				return e
			}
			if _, e := tx.Exec(`UPDATE invites SET consumed_at=?,consumed_by=? WHERE token_hash=?`, now(), u.ID, c.InviteHash); e != nil {
				return e
			}
			var e error
			token, e = a.makeVerifiedSession(tx, u)
			if e != nil {
				return e
			}
			return audit(tx, u.ID, "auth.join", u.ID)
		})
		if e != nil {
			return e
		}
		a.sessionResponse(w, u, token)
		return nil
	})
	a.route("GET /api/v1/account/identities", "read", a.listIdentities)
	for _, purpose := range []string{"bind", "replace"} {
		a.route("POST /api/v1/account/identities/"+purpose, "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
			if purpose == "replace" {
				if e := a.requireReauth(r, u); e != nil {
					return e
				}
			}
			var p codeProof
			if e := decode(w, r, &p); e != nil {
				return e
			}
			e := a.withChallenge(r, p, purpose, func(tx *sql.Tx, c challenge) error {
				if c.UserID != u.ID {
					return fail(403, "FORBIDDEN", "验证码不属于当前账号")
				}
				if purpose == "replace" {
					if _, e := tx.Exec(`DELETE FROM user_identities WHERE user_id=? AND kind=?`, u.ID, c.Kind); e != nil {
						return e
					}
				}
				if e := insertIdentity(tx, u.ID, c); e != nil {
					return e
				}
				if e := revokeCredentials(tx, u.ID); e != nil {
					return e
				}
				return audit(tx, u.ID, "identity."+purpose, u.ID)
			})
			if e != nil {
				return e
			}
			writeJSON(w, 200, map[string]bool{"ok": true, "login_required": true})
			return nil
		})
	}
	a.route("POST /api/v1/account/reauth", "read", a.reauthenticate)
	a.route("DELETE /api/v1/account/identities/{id}", "read", a.removeIdentity)
	a.route("PUT /api/v1/account/password", "read", a.changePassword)
	a.route("GET /api/v1/account/sessions", "read", a.listSessions)
	a.route("DELETE /api/v1/account/sessions/{id}", "read", a.removeSession)
	a.route("DELETE /api/v1/account/sessions", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if _, e := a.store.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, u.ID); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
	a.route("GET /api/v1/system/auth-status", "system", func(w http.ResponseWriter, r *http.Request, u *User) error {
		phone, email := false, false
		if a.auth != nil {
			phone = a.auth.cfg.Phone != nil
			email = a.auth.cfg.Email != nil
		}
		writeJSON(w, 200, map[string]any{"phone_configured": phone, "email_configured": email, "delivery_verified": false, "storage": a.storageName()})
		return nil
	})
}
func insertIdentity(tx *sql.Tx, uid string, c challenge) error {
	_, e := tx.Exec(`INSERT INTO user_identities(id,user_id,kind,address_cipher,lookup_hash,verified_at) VALUES(?,?,?,?,?,?)`, newID(), uid, c.Kind, c.Cipher, c.Lookup, now())
	if e != nil && strings.Contains(e.Error(), "UNIQUE") {
		return fail(409, "IDENTITY_UNAVAILABLE", "该联系方式无法绑定，请使用已有账号登录")
	}
	return e
}
func (a *App) listIdentities(w http.ResponseWriter, r *http.Request, u *User) error {
	if e := a.authReady(); e != nil {
		return e
	}
	rows, e := a.store.DB.Query(`SELECT id,kind,address_cipher,verified_at FROM user_identities WHERE user_id=? ORDER BY kind`, u.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, kind, date string
		var b []byte
		if e = rows.Scan(&id, &kind, &b, &date); e != nil {
			return e
		}
		target, e := a.auth.decrypt(kind, b)
		if e != nil {
			return e
		}
		items = append(items, map[string]string{"id": id, "kind": kind, "masked": maskedIdentity(kind, target), "verified_at": date})
	}
	if e = rows.Err(); e != nil {
		return e
	}
	writeJSON(w, 200, map[string]any{"items": items})
	return nil
}
func (a *App) reauthenticate(w http.ResponseWriter, r *http.Request, u *User) error {
	var b struct {
		codeProof
		Password string `json:"password"`
	}
	if e := decode(w, r, &b); e != nil {
		return e
	}
	if b.Password != "" {
		return fail(410, "EMAIL_CODE_REQUIRED", "请使用邮箱验证码验证身份")
	}
	_, token := a.session(r)
	if e := a.withChallenge(r, b.codeProof, "reauth", func(tx *sql.Tx, c challenge) error {
		if c.UserID != u.ID || c.Kind != "email" {
			return fail(403, "FORBIDDEN", "验证未通过")
		}
		if _, e := tx.Exec(`UPDATE sessions SET reauthenticated_at=? WHERE token_hash=?`, now(), hashToken(token)); e != nil {
			return e
		}
		return audit(tx, u.ID, "auth.reauth", u.ID)
	}); e != nil {
		return e
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}
func (a *App) removeIdentity(w http.ResponseWriter, r *http.Request, u *User) error {
	if e := a.requireReauth(r, u); e != nil {
		return e
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	tx, e := a.store.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var count int
	if e = tx.QueryRow(`SELECT count(*) FROM user_identities WHERE user_id=?`, u.ID).Scan(&count); e != nil {
		return e
	}
	if count <= 1 {
		return fail(409, "LAST_IDENTITY", "至少保留一种可用登录方式")
	}
	if _, e = tx.Exec(`DELETE FROM user_identities WHERE id=? AND user_id=?`, r.PathValue("id"), u.ID); e != nil {
		return e
	}
	if e = revokeCredentials(tx, u.ID); e != nil {
		return e
	}
	if e = audit(tx, u.ID, "identity.remove", r.PathValue("id")); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}
func (a *App) changePassword(w http.ResponseWriter, r *http.Request, u *User) error {
	return fail(410, "EMAIL_CODE_REQUIRED", "当前账号仅使用邮箱验证码，无需设置密码")
}
func (a *App) listSessions(w http.ResponseWriter, r *http.Request, u *User) error {
	_, t := a.session(r)
	rows, e := a.store.DB.Query(`SELECT token_hash,created_at,expires_at FROM sessions WHERE user_id=? AND expires_at>? ORDER BY created_at DESC LIMIT 50`, u.ID, now())
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, created, expires string
		if e = rows.Scan(&id, &created, &expires); e != nil {
			return e
		}
		items = append(items, map[string]any{"id": id, "created_at": created, "expires_at": expires, "current": id == hashToken(t)})
	}
	if e = rows.Err(); e != nil {
		return e
	}
	writeJSON(w, 200, map[string]any{"items": items})
	return nil
}
func (a *App) removeSession(w http.ResponseWriter, r *http.Request, u *User) error {
	_, e := a.store.DB.Exec(`DELETE FROM sessions WHERE token_hash=? AND user_id=?`, r.PathValue("id"), u.ID)
	if e != nil {
		return e
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func revokeCredentials(tx *sql.Tx, id string) error {
	if _, e := tx.Exec(`DELETE FROM sessions WHERE user_id=?`, id); e != nil {
		return e
	}
	_, e := tx.Exec(`UPDATE auth_challenges SET state='revoked' WHERE user_id=? AND state IN ('pending','sent','decoy','unknown')`, id)
	return e
}

func (a *App) sessionAge() time.Duration {
	if a.auth != nil {
		return time.Duration(a.auth.cfg.SessionDays) * 24 * time.Hour
	}
	return 30 * 24 * time.Hour
}
func (a *App) reauthWindow() time.Duration {
	if a.auth != nil {
		return time.Duration(a.auth.cfg.ReauthSeconds) * time.Second
	}
	return 5 * time.Minute
}

func (a *App) clientIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	peer, e := netip.ParseAddr(host)
	if e != nil {
		return host
	}
	peer = peer.Unmap()
	trusted := func(ip netip.Addr) bool {
		if a.auth == nil {
			return false
		}
		for _, p := range a.auth.cfg.TrustedProxies {
			if p.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer.String()
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(chain) > 10 {
		return peer.String()
	}
	for i := len(chain) - 1; i >= 0; i-- {
		ip, e := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if e != nil {
			return peer.String()
		}
		peer = ip.Unmap()
		if !trusted(peer) {
			return peer.String()
		}
	}
	return peer.String()
}
