package wiki

import "net/http"

func (a *App) memberRoutes() {
	a.route("GET /api/v1/admin/members", "admin", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		cursor := r.URL.Query().Get("cursor")
		rows, e := a.store.DB.Query(`SELECT id,name,login,role,disabled FROM users WHERE (?='' OR id>?) ORDER BY id LIMIT 21`, cursor, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var u User
			var disabled bool
			if e = rows.Scan(&u.ID, &u.Name, &u.Login, &u.Role, &disabled); e != nil {
				return e
			}
			out = append(out, map[string]any{"id": u.ID, "name": u.Name, "login": u.Login, "role": u.Role, "disabled": disabled})
		}
		if e = rows.Err(); e != nil {
			return e
		}
		next := ""
		if len(out) > 20 {
			out = out[:20]
			next = out[len(out)-1]["id"].(string)
		}
		writeJSON(w, 200, map[string]any{"items": out, "next_cursor": next})
		return nil
	})
	a.route("PATCH /api/v1/admin/members/{id}", "admin", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		var b struct {
			Role     string `json:"role"`
			Disabled bool   `json:"disabled"`
		}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		if b.Role != "admin" && b.Role != "editor" && b.Role != "viewer" {
			return fail(400, "VALIDATION_ERROR", "成员角色无效")
		}
		tx, e := a.store.DB.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		var oldRole string
		var disabled bool
		var hashLength int
		if e = tx.QueryRow(`SELECT role,disabled,length(password_hash) FROM users WHERE id=?`, r.PathValue("id")).Scan(&oldRole, &disabled, &hashLength); e != nil {
			return fail(404, "NOT_FOUND", "成员不存在")
		}
		if b.Role == "admin" && hashLength == 0 && !(a.cfg.Demo && r.PathValue("id") == "demo-editor") {
			return fail(400, "VALIDATION_ERROR", "旧站署名账号没有登录凭证，请邀请一个可登录的成员再授予管理员权限")
		}
		if oldRole == "admin" && !disabled && (b.Role != "admin" || b.Disabled) {
			var count int
			if e = tx.QueryRow(`SELECT count(*) FROM users WHERE role='admin' AND disabled=0 AND (length(password_hash)>0 OR (? AND id='demo-editor'))`, a.cfg.Demo).Scan(&count); e != nil {
				return e
			}
			if count <= 1 {
				return fail(409, "LAST_ADMIN", "请至少保留一位可登录的管理员")
			}
		}
		if _, e = tx.Exec(`UPDATE users SET role=?,disabled=? WHERE id=?`, b.Role, b.Disabled, r.PathValue("id")); e != nil {
			return e
		}
		if b.Disabled {
			if _, e = tx.Exec(`DELETE FROM sessions WHERE user_id=?`, r.PathValue("id")); e != nil {
				return e
			}
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
}
