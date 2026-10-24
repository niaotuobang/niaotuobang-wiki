package wiki

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	DataDir, BaseURL string
	Demo             bool
	UI               fs.FS
	Auth             *AuthConfig
	OSS              *OSSConfig
}
type App struct {
	store      *Store
	cfg        Config
	mux        *http.ServeMux
	uploadMu   sync.Mutex
	authMu     sync.Mutex
	setupToken string
	oss        *OSSStorage
	auth       *authService
}

type endpoint func(http.ResponseWriter, *http.Request, *User) error

func Open(cfg Config) (*App, error) {
	if cfg.Demo && cfg.OSS != nil {
		return nil, fail(400, "VALIDATION_ERROR", "演示模式只使用独立的本地数据和媒体，请移除 OSS 配置")
	}
	s, err := openStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	a := &App{store: s, cfg: cfg, mux: http.NewServeMux()}
	if cfg.OSS != nil {
		a.oss, err = newOSS(*cfg.OSS)
		if err != nil {
			s.DB.Close()
			return nil, err
		}
	}
	if cfg.Demo {
		if err = a.seedDemo(); err != nil {
			s.DB.Close()
			return nil, err
		}
	}
	var users int
	s.DB.QueryRow(`SELECT count(*) FROM users`).Scan(&users)
	if users == 0 {
		path := filepath.Join(s.DataDir, "setup-token")
		b, e := os.ReadFile(path)
		if errors.Is(e, os.ErrNotExist) {
			b = []byte(newID())
			e = os.WriteFile(path, b, 0600)
		}
		if e != nil {
			s.DB.Close()
			return nil, e
		}
		a.setupToken = strings.TrimSpace(string(b))
	}
	if _, err = s.DB.Exec(`CREATE TABLE IF NOT EXISTS invites(token_hash TEXT PRIMARY KEY,role TEXT NOT NULL,expires_at TEXT NOT NULL)`); err != nil {
		s.DB.Close()
		return nil, err
	}
	if cfg.Auth != nil {
		if err = a.initAuth(*cfg.Auth); err != nil {
			s.DB.Close()
			return nil, err
		}
	}
	a.routes()
	a.identityRoutes()
	return a, nil
}
func (a *App) Close() error { return a.store.DB.Close() }
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: https:; media-src 'self' blob: https:; connect-src 'self' https:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/media/") {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				allowed := r.Host
				if a.cfg.BaseURL != "" {
					if base, e := url.Parse(a.cfg.BaseURL); e == nil {
						allowed = base.Host
					}
				}
				if e != nil || u.Host != allowed {
					writeError(w, fail(403, "FORBIDDEN", "请求来源不受信任"))
					return
				}
			}
		}
		a.mux.ServeHTTP(w, r)
	})
}
func hashToken(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func csrfToken(t string) string { return hashToken("wiki-csrf:" + t) }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	var api *APIError
	if !errors.As(err, &api) {
		log.Printf("request failed: %T", err)
		api = &APIError{500, "INTERNAL_ERROR", "操作失败，请稍后重试"}
	}
	writeJSON(w, api.Status, map[string]any{"error": map[string]string{"code": api.Code, "message": api.Message}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fail(415, "VALIDATION_ERROR", "请使用 JSON 请求")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fail(400, "VALIDATION_ERROR", "请求格式或字段不正确")
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return fail(400, "VALIDATION_ERROR", "请求只能包含一个对象")
	}
	return nil
}
func (a *App) session(r *http.Request) (*User, string) {
	token := ""
	if c, e := r.Cookie("wiki_session"); e == nil {
		token = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if token == "" {
		return nil, ""
	}
	var u User
	err := a.store.DB.QueryRow(`SELECT u.id,u.name,u.login,u.access_role,u.can_edit,coalesce(u.avatar_media_id,'') FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.disabled=0`, hashToken(token), now()).Scan(&u.ID, &u.Name, &u.Login, &u.Role, &u.CanEdit, &u.AvatarMediaID)
	if err != nil {
		return nil, ""
	}
	return &u, token
}
func (a *App) publicRead() bool { return false }
func (a *App) route(pattern, permission string, fn endpoint) {
	a.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		u, token := a.session(r)
		if permission != "public" && u == nil {
			writeError(w, fail(401, "UNAUTHENTICATED", "请先登录群 Wiki"))
			return
		}
		if permission == "edit" && (u == nil || !u.Editable()) || permission == "admin" && (u == nil || !u.IsAdmin()) || permission == "system" && (u == nil || u.Role != "system_admin") {
			writeError(w, fail(403, "FORBIDDEN", "当前账号没有操作权限"))
			return
		}
		if token != "" && r.Method != "GET" && r.Method != "HEAD" && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfToken(token))) != 1 {
				writeError(w, fail(403, "FORBIDDEN", "页面会话已变化，请刷新后重试"))
				return
			}
		}
		if err := fn(w, r, u); err != nil {
			writeError(w, err)
		}
	})
}
func (a *App) login(w http.ResponseWriter, u User) error {
	token, e := a.makeSession(a.store.DB, u)
	if e != nil {
		return e
	}
	a.sessionResponse(w, u, token)
	return nil
}

func (a *App) routes() {
	a.route("GET /api/v1/community", "read", a.community)
	a.route("PATCH /api/v1/account/profile", "read", a.updateProfile)
	a.route("GET /api/v1/me", "public", func(w http.ResponseWriter, r *http.Request, u *User) error {
		a.authMu.Lock()
		defer a.authMu.Unlock()
		_, t := a.session(r)
		csrf := ""
		if t != "" {
			csrf = csrfToken(t)
		}
		writeJSON(w, 200, map[string]any{"user": u, "csrf_token": csrf, "demo": a.cfg.Demo, "setup_needed": a.setupToken != "", "public_read": a.publicRead()})
		return nil
	})
	a.route("POST /api/v1/auth/demo", "public", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		if !a.cfg.Demo {
			return fail(404, "NOT_FOUND", "入口不存在")
		}
		var b struct{}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		var u User
		var disabled bool
		if e := a.store.DB.QueryRow(`SELECT id,name,login,access_role,can_edit,disabled,coalesce(avatar_media_id,'') FROM users WHERE id='demo-editor'`).Scan(&u.ID, &u.Name, &u.Login, &u.Role, &u.CanEdit, &disabled, &u.AvatarMediaID); e != nil {
			return e
		}
		if disabled {
			return fail(403, "FORBIDDEN", "演示账号已停用")
		}
		return a.login(w, u)
	})
	a.route("POST /api/v1/auth/login", "public", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		return fail(410, "EMAIL_CODE_REQUIRED", "请使用邮箱验证码登录")
	})
	a.route("POST /api/v1/auth/setup", "public", a.setupWithEmail)
	a.route("POST /api/v1/auth/logout", "read", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		_, token := a.session(r)
		if _, e := a.store.DB.Exec(`DELETE FROM sessions WHERE token_hash=?`, hashToken(token)); e != nil {
			return e
		}
		http.SetCookie(w, &http.Cookie{Name: "wiki_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
	a.route("GET /api/v1/articles", "read", a.listArticles)
	a.route("GET /api/v1/timeline", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		v := r.URL.Query()
		v.Set("timeline", "1")
		r.URL.RawQuery = v.Encode()
		return a.listArticles(w, r, u)
	})
	a.route("GET /api/v1/articles/random", "read", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		var id string
		exclude := r.URL.Query().Get("exclude")
		if e := a.store.DB.QueryRow(`SELECT id FROM articles WHERE status='published' AND deleted_at='' AND id<>? ORDER BY random() LIMIT 1`, exclude).Scan(&id); e != nil {
			if e != sql.ErrNoRows {
				return e
			}
			if exclude != "" {
				return fail(404, "NOT_FOUND", "暂时没有其他文章了")
			}
			return fail(404, "NOT_FOUND", "还没有可阅读的文章")
		}
		writeJSON(w, 200, map[string]string{"id": id})
		return nil
	})
	a.route("GET /api/v1/articles/{id}", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		v, e := getArticle(a.store.DB, r.PathValue("id"))
		if e != nil {
			return e
		}
		if !canRead(v, u) {
			return fail(404, "NOT_FOUND", "文章不存在")
		}
		writeJSON(w, 200, v)
		return nil
	})
	a.route("POST /api/v1/articles", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		var b WriteArticle
		if e := decode(w, r, &b); e != nil {
			return e
		}
		v, e := a.store.saveArticle("", b, *u, true)
		if e != nil {
			return e
		}
		writeJSON(w, 201, v)
		return nil
	})
	a.route("PUT /api/v1/articles/{id}", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		var b WriteArticle
		if e := decode(w, r, &b); e != nil {
			return e
		}
		v, e := a.store.saveArticle(r.PathValue("id"), b, *u, false)
		if e != nil {
			return e
		}
		writeJSON(w, 200, v)
		return nil
	})
	for _, restore := range []bool{false, true} {
		method, path := "DELETE", "/api/v1/articles/{id}"
		if restore {
			method, path = "POST", "/api/v1/articles/{id}/restore"
		}
		a.route(method+" "+path, "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
			v, e := getArticle(a.store.DB, r.PathValue("id"))
			if e != nil {
				return e
			}
			if v.Status == "draft" && v.AuthorID != u.ID {
				return fail(404, "NOT_FOUND", "文章不存在")
			}
			deleted := now()
			if restore {
				deleted = ""
			}
			_, e = a.store.DB.Exec(`UPDATE articles SET deleted_at=? WHERE id=?`, deleted, v.ID)
			if e != nil {
				return e
			}
			if _, e = a.store.DB.Exec(`UPDATE legacy_article_metadata SET deletion_time_unknown=0 WHERE article_id=?`, v.ID); e != nil {
				return e
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return nil
		})
	}
	a.route("GET /api/v1/articles/{id}/versions", "read", a.versions)
	a.route("GET /api/v1/articles/{id}/versions/{version}", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if e := a.readPublished(r.PathValue("id"), u); e != nil {
			return e
		}
		v, e := a.version(r.PathValue("id"), r.PathValue("version"))
		if e != nil {
			return e
		}
		writeJSON(w, 200, v)
		return nil
	})
	a.route("POST /api/v1/articles/{id}/versions/{version}/restore", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if e := a.readPublished(r.PathValue("id"), u); e != nil {
			return e
		}
		var b struct {
			Revision int  `json:"expected_revision"`
			Ignore   bool `json:"ignore_missing_tags"`
		}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		old, e := a.version(r.PathValue("id"), r.PathValue("version"))
		if e != nil {
			return e
		}
		body := fromArticle(old)
		body.ExpectedRevision = b.Revision
		body.ChangeSummary = "恢复历史版本 " + r.PathValue("version")
		body.TagIDs = nil
		for _, tag := range old.Tags {
			var n int
			a.store.DB.QueryRow(`SELECT count(*) FROM tags WHERE id=?`, tag.ID).Scan(&n)
			if n == 0 {
				if !b.Ignore {
					return fail(422, "MISSING_TAGS", "旧版本含已删除的标签，请明确选择是否忽略后恢复")
				}
				continue
			}
			body.TagIDs = append(body.TagIDs, tag.ID)
		}
		v, e := a.store.saveArticle(old.ID, body, *u, false)
		if e != nil {
			return e
		}
		writeJSON(w, 200, v)
		return nil
	})
	a.route("GET /api/v1/articles/{id}/backlinks", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if e := a.readPublished(r.PathValue("id"), u); e != nil {
			return e
		}
		rows, e := a.store.DB.Query(`SELECT a.id,a.title,coalesce(nullif(l.context,''),a.plain_text),l.paragraph_id FROM article_links l JOIN articles a ON a.id=l.source_id WHERE l.target_id=? AND a.status='published' AND a.deleted_at='' AND (?='' OR a.id<?) ORDER BY a.id DESC LIMIT 21`, r.PathValue("id"), r.URL.Query().Get("cursor"), r.URL.Query().Get("cursor"))
		if e != nil {
			return e
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var id, title, text, paragraphID string
			if e := rows.Scan(&id, &title, &text, &paragraphID); e != nil {
				return e
			}
			runes := []rune(text)
			if len(runes) > 300 {
				text = string(runes[:300]) + "…"
			}
			out = append(out, map[string]string{"id": id, "title": title, "context": text, "paragraph_id": paragraphID})
		}
		if e := rows.Err(); e != nil {
			return e
		}
		next := ""
		if len(out) > 20 {
			out = out[:20]
			next = out[len(out)-1]["id"]
		}
		writeJSON(w, 200, map[string]any{"items": out, "next_cursor": next})
		return nil
	})
	a.draftRoutes()
	a.tagRoutes()
	a.adminRoutes()
	a.memberRoutes()
	a.mediaRoutes()
	a.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	a.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if e := a.store.DB.PingContext(r.Context()); e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	a.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, fail(404, "NOT_FOUND", "接口不存在")) })
	a.mux.HandleFunc("/", a.static)
}

type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

func prepareUser(name, login, password, role string) (User, []byte, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	name = strings.TrimSpace(name)
	if len(password) < 10 || len(password) > 72 || len(name) == 0 || len([]rune(name)) > 40 || len(login) < 3 || len(login) > 100 {
		return User{}, nil, fail(400, "VALIDATION_ERROR", "昵称必填，账号至少 3 字符，密码需要 10 至 72 字节")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	r, edit := canonicalRole(role)
	return User{ID: newID(), Name: name, Login: login, Role: r, CanEdit: edit}, hash, e
}
func insertUser(db execer, u User, hash []byte) error {
	_, e := db.Exec(`INSERT INTO users(id,name,login,password_hash,role,access_role,can_edit,created_at) VALUES(?,?,?,?,?,?,?,?)`, u.ID, u.Name, u.Login, hash, legacyRole(u), u.Role, u.CanEdit, now())
	if e != nil && strings.Contains(e.Error(), "UNIQUE") {
		return fail(409, "ACCOUNT_EXISTS", "账号已经存在")
	}
	return e
}
func (a *App) createUser(name, login, password, role string) (User, error) {
	u, hash, e := prepareUser(name, login, password, role)
	if e != nil {
		return User{}, e
	}
	e = insertUser(a.store.DB, u, hash)
	return u, e
}
func (a *App) listArticles(w http.ResponseWriter, r *http.Request, _ *User) error {
	v, e := a.store.list(r.URL.Query())
	if e != nil {
		return e
	}
	writeJSON(w, 200, v)
	return nil
}
func (a *App) readPublished(id string, u *User) error {
	v, e := getArticle(a.store.DB, id)
	if e != nil {
		return e
	}
	if !canRead(v, u) || v.Status != "published" {
		return fail(404, "NOT_FOUND", "文章不存在")
	}
	return nil
}
func (a *App) version(id, version string) (Article, error) {
	var s string
	e := a.store.DB.QueryRow(`SELECT snapshot FROM article_versions WHERE article_id=? AND revision=?`, id, version).Scan(&s)
	if errors.Is(e, sql.ErrNoRows) {
		return Article{}, fail(404, "NOT_FOUND", "版本不存在")
	}
	if e != nil {
		return Article{}, e
	}
	var v Article
	e = json.Unmarshal([]byte(s), &v)
	if e != nil {
		return v, e
	}
	var avatar string
	err := a.store.DB.QueryRow(`SELECT coalesce(avatar_media_id,'') FROM users WHERE id=?`, v.UpdatedBy).Scan(&avatar)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, err
	}
	v.UpdatedByAvatar = avatar
	return v, nil
}
func (a *App) versions(w http.ResponseWriter, r *http.Request, u *User) error {
	if e := a.readPublished(r.PathValue("id"), u); e != nil {
		return e
	}
	return a.listVersions(w, r)
}
func (a *App) listVersions(w http.ResponseWriter, r *http.Request) error {
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		if n, e := strconv.Atoi(value); e == nil && n > 0 {
			limit = n
		}
	}
	if limit > 50 {
		limit = 50
	}
	cursor := int(^uint(0) >> 1)
	if value := r.URL.Query().Get("cursor"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 {
			return fail(400, "VALIDATION_ERROR", "版本游标无效")
		}
		cursor = n
	}
	rows, e := a.store.DB.Query(`SELECT json_set(json_remove(v.snapshot,'$.document','$.plain_text'),'$.updated_by_avatar',coalesce(u.avatar_media_id,'')) FROM article_versions v LEFT JOIN users u ON u.id=json_extract(v.snapshot,'$.updated_by') WHERE v.article_id=? AND v.revision<? ORDER BY v.revision DESC LIMIT ?`, r.PathValue("id"), cursor, limit+1)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var raw string
		if e := rows.Scan(&raw); e != nil {
			return e
		}
		var v map[string]any
		if e = json.Unmarshal([]byte(raw), &v); e != nil {
			return e
		}
		items = append(items, v)
	}
	if e := rows.Err(); e != nil {
		return e
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.Itoa(number(items[len(items)-1]["revision"]))
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
	return nil
}
func (a *App) draftRoutes() {
	a.route("GET /api/v1/drafts", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if u == nil {
			return fail(401, "UNAUTHENTICATED", "请先登录")
		}
		v := r.URL.Query()
		cursorTime, cursorID := "", ""
		if value := v.Get("cursor"); value != "" {
			var pair []string
			if json.Unmarshal([]byte(value), &pair) != nil || len(pair) != 2 {
				return fail(400, "VALIDATION_ERROR", "草稿游标无效")
			}
			cursorTime, cursorID = pair[0], pair[1]
		}
		rows, e := a.store.DB.Query(`SELECT d.article_id,json_extract(d.body,'$.title'),d.updated_at FROM article_drafts d JOIN articles a ON a.id=d.article_id WHERE d.user_id=? AND a.deleted_at='' AND (?='' OR d.updated_at<? OR (d.updated_at=? AND d.article_id<?)) ORDER BY d.updated_at DESC,d.article_id DESC LIMIT 21`, u.ID, cursorTime, cursorTime, cursorTime, cursorID)
		if e != nil {
			return e
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, title, ts string
			if e = rows.Scan(&id, &title, &ts); e != nil {
				return e
			}
			out = append(out, map[string]any{"id": id, "title": title, "updated_at": ts})
		}
		if e = rows.Err(); e != nil {
			return e
		}
		next := ""
		if len(out) > 20 {
			out = out[:20]
			last := out[len(out)-1]
			b, _ := json.Marshal([]any{last["updated_at"], last["id"]})
			next = string(b)
		}
		writeJSON(w, 200, map[string]any{"items": out, "next_cursor": next})
		return nil
	})
	a.route("GET /api/v1/drafts/{id}", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if u == nil {
			return fail(401, "UNAUTHENTICATED", "请先登录")
		}
		var body string
		e := a.store.DB.QueryRow(`SELECT body FROM article_drafts d JOIN articles a ON a.id=d.article_id WHERE d.article_id=? AND d.user_id=? AND a.deleted_at=''`, r.PathValue("id"), u.ID).Scan(&body)
		if e != nil {
			return fail(404, "NOT_FOUND", "没有保存的草稿")
		}
		var b WriteArticle
		if e = json.Unmarshal([]byte(body), &b); e != nil {
			return e
		}
		writeJSON(w, 200, b)
		return nil
	})
	a.route("PUT /api/v1/drafts/{id}", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		var b WriteArticle
		if e := decode(w, r, &b); e != nil {
			return e
		}
		if _, e := validateWrite(b, false); e != nil {
			return e
		}
		b.Document = canonicalDocument(b.Document)
		v, e := getArticle(a.store.DB, r.PathValue("id"))
		if e != nil {
			return e
		}
		if !canRead(v, u) {
			return fail(404, "NOT_FOUND", "文章不存在")
		}
		s, _ := json.Marshal(b)
		_, e = a.store.DB.Exec(`INSERT INTO article_drafts(article_id,user_id,body,updated_at) VALUES(?,?,?,?) ON CONFLICT(article_id,user_id) DO UPDATE SET body=excluded.body,updated_at=excluded.updated_at`, v.ID, u.ID, string(s), now())
		if e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
	a.route("DELETE /api/v1/drafts/{id}", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		_, e := a.store.DB.Exec(`DELETE FROM article_drafts WHERE article_id=? AND user_id=?`, r.PathValue("id"), u.ID)
		if e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
}
func (a *App) tagRoutes() {
	a.route("GET /api/v1/tags", "read", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		v, e := a.store.tags()
		if e != nil {
			return e
		}
		writeJSON(w, 200, map[string]any{"items": v})
		return nil
	})
	a.route("GET /api/v1/tags/{id}", "read", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		tags, e := a.store.tags()
		if e != nil {
			return e
		}
		for _, t := range tags {
			if t.ID == r.PathValue("id") {
				writeJSON(w, 200, t)
				return nil
			}
		}
		return fail(404, "NOT_FOUND", "标签不存在")
	})
	a.route("POST /api/v1/tags", "edit", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		var b Tag
		if e := decode(w, r, &b); e != nil {
			return e
		}
		v, e := a.store.saveTag("", b, true)
		if e != nil {
			return e
		}
		writeJSON(w, 201, v)
		return nil
	})
	a.route("PATCH /api/v1/tags/{id}", "edit", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		var b Tag
		if e := decode(w, r, &b); e != nil {
			return e
		}
		v, e := a.store.saveTag(r.PathValue("id"), b, false)
		if e != nil {
			return e
		}
		writeJSON(w, 200, v)
		return nil
	})
	a.route("DELETE /api/v1/tags/{id}", "admin", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		if _, e := a.store.DB.Exec(`DELETE FROM tags WHERE id=?`, r.PathValue("id")); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
}
func (a *App) adminRoutes() {
	a.trashRoutes()
	a.route("GET /api/v1/admin", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		counts := map[string]int{}
		for key, query := range map[string]string{"articles": `SELECT count(*) FROM articles WHERE status='published' AND deleted_at=''`, "members": `SELECT count(*) FROM users`, "media": `SELECT count(*) FROM media WHERE state='ready'`} {
			var n int
			if e := a.store.DB.QueryRow(query).Scan(&n); e != nil {
				return e
			}
			counts[key] = n
		}
		rows, e := a.store.DB.Query(`SELECT id,title,deleted_at FROM articles WHERE deleted_at<>'' AND (status='published' OR author_id=?) ORDER BY deleted_at DESC LIMIT 100`, u.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		trash := []map[string]string{}
		for rows.Next() {
			var id, title, date string
			rows.Scan(&id, &title, &date)
			trash = append(trash, map[string]string{"id": id, "title": title, "deleted_at": date})
		}
		if e := rows.Err(); e != nil {
			return e
		}
		rows.Close()
		writeJSON(w, 200, map[string]any{"counts": counts, "trash": trash, "public_read": a.publicRead(), "storage": a.storageName()})
		return rows.Err()
	})
	a.route("PATCH /api/v1/admin/settings", "system", func(w http.ResponseWriter, r *http.Request, u *User) error {
		return fail(400, "PRIVATE_WIKI", "此 Wiki 必须登录，不支持公开阅读")
	})
	a.inviteRoutes()
	a.route("POST /api/v1/auth/register", "public", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		return fail(410, "VERIFICATION_REQUIRED", "请使用邀请凭证和手机或邮箱验证码加入")
	})
	a.route("GET /api/v1/admin/backup", "system", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if e := a.requireReauth(r, u); e != nil {
			return e
		}
		return a.downloadBackup(w, r, u)
	})
}

var pagePath = regexp.MustCompile(`^/(|tags|tags/[A-Za-z0-9_-]+|articles/new|articles/[A-Za-z0-9_-]+(?:/edit|/history)?|timeline|search|drafts|account|admin|community|trash|trash/[A-Za-z0-9_-]+)$`)

func (a *App) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.NotFound(w, r)
		return
	}
	if a.cfg.UI == nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		info, e := fs.Stat(a.cfg.UI, strings.TrimPrefix(r.URL.Path, "/"))
		if e != nil || info.IsDir() {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.FileServer(http.FS(a.cfg.UI)).ServeHTTP(w, r)
		return
	}
	if !pagePath.MatchString(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	b, e := fs.ReadFile(a.cfg.UI, "index.html")
	if e != nil {
		http.Error(w, "网页资源尚未构建", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}
