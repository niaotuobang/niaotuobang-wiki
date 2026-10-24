package wiki

import (
	"database/sql"
	"net/http"
)

func (a *App) memberRoutes() {
	a.route("GET /api/v1/admin/members", "admin", func(w http.ResponseWriter, r *http.Request, _ *User) error {
		cursor := r.URL.Query().Get("cursor")
		rows, e := a.store.DB.Query(`SELECT id,name,login,access_role,can_edit,disabled FROM users WHERE (?='' OR id>?) ORDER BY id LIMIT 21`, cursor, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var u User
			var disabled bool
			if e = rows.Scan(&u.ID, &u.Name, &u.Login, &u.Role, &u.CanEdit, &disabled); e != nil {
				return e
			}
			out = append(out, map[string]any{"id": u.ID, "name": u.Name, "login": u.Login, "role": u.Role, "can_edit": u.CanEdit, "disabled": disabled})
		}
		if e = rows.Err(); e != nil {
			return e
		}
		next := ""
		if len(out) > 20 {
			out = out[:20]
			next = out[19]["id"].(string)
		}
		writeJSON(w, 200, map[string]any{"items": out, "next_cursor": next})
		return nil
	})
	a.route("PATCH /api/v1/admin/members/{id}", "admin", func(w http.ResponseWriter, r *http.Request, actor *User) error {
		var b struct {
			Role     string `json:"role"`
			CanEdit  *bool  `json:"can_edit"`
			Disabled bool   `json:"disabled"`
		}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		if b.Role != "system_admin" && b.Role != "admin" && b.Role != "user" && b.Role != "editor" && b.Role != "viewer" {
			return fail(400, "VALIDATION_ERROR", "成员角色无效")
		}
		role, edit := canonicalRole(b.Role)
		if b.CanEdit != nil && role == "user" {
			edit = *b.CanEdit
		}
		id := r.PathValue("id")
		var old string
		var disabled bool
		var hashLen int
		e := a.store.DB.QueryRow(`SELECT access_role,disabled,length(password_hash) FROM users WHERE id=?`, id).Scan(&old, &disabled, &hashLen)
		if e == sql.ErrNoRows {
			return fail(404, "NOT_FOUND", "成员不存在")
		}
		if e != nil {
			return e
		}
		if actor.Role != "system_admin" && (old != "user" || role != "user" || actor.ID == id) {
			return fail(403, "FORBIDDEN", "管理员只能管理普通用户")
		}
		if old == "system_admin" && !disabled && (role != "system_admin" || b.Disabled) {
			var count int
			if e = a.store.DB.QueryRow(`SELECT count(*) FROM users WHERE access_role='system_admin' AND disabled=0 AND (length(password_hash)>0 OR (? AND id='demo-editor'))`, a.cfg.Demo).Scan(&count); e != nil {
				return e
			}
			if count <= 1 {
				return fail(409, "LAST_ADMIN", "请至少保留一位可登录的系统管理员")
			}
		}
		if role == "system_admin" && hashLen == 0 && !(a.cfg.Demo && id == "demo-editor") {
			return fail(400, "PASSWORD_REQUIRED", "系统管理员必须先设置可用密码")
		}
		if role != "user" || old != "user" {
			if e = a.requireReauth(r, actor); e != nil {
				return e
			}
		}
		a.authMu.Lock()
		defer a.authMu.Unlock()
		tx, e := a.store.DB.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		// Recheck the last-system-admin invariant in the same transaction as the update.
		var current string
		var currentDisabled bool
		if e = tx.QueryRow(`SELECT access_role,disabled FROM users WHERE id=?`, id).Scan(&current, &currentDisabled); e != nil {
			return e
		}
		if current != old || currentDisabled != disabled {
			return fail(409, "CONFLICT", "成员状态已变化，请刷新")
		}
		if old == "system_admin" && !disabled && (role != "system_admin" || b.Disabled) {
			var count int
			if e = tx.QueryRow(`SELECT count(*) FROM users WHERE access_role='system_admin' AND disabled=0 AND (length(password_hash)>0 OR (? AND id='demo-editor'))`, a.cfg.Demo).Scan(&count); e != nil {
				return e
			}
			if count <= 1 {
				return fail(409, "LAST_ADMIN", "请至少保留一位可登录的系统管理员")
			}
		}
		target := User{Role: role, CanEdit: edit}
		if _, e = tx.Exec(`UPDATE users SET role=?,access_role=?,can_edit=?,disabled=? WHERE id=?`, legacyRole(target), role, edit, b.Disabled, id); e != nil {
			return e
		}
		if e = revokeCredentials(tx, id); e != nil {
			return e
		}
		if e = audit(tx, actor.ID, "member.update", id); e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
}

// PromoteSystemAdmin is an explicit local upgrade operation, never a web endpoint.
func (a *App) PromoteSystemAdmin(id string) error {
	tx, e := a.store.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var role string
	var length int
	var disabled bool
	if e = tx.QueryRow(`SELECT access_role,length(password_hash),disabled FROM users WHERE id=?`, id).Scan(&role, &length, &disabled); e != nil {
		return fail(400, "VALIDATION_ERROR", "指定的升级账号不存在")
	}
	if role != "admin" || length == 0 || disabled {
		return fail(400, "VALIDATION_ERROR", "升级账号必须是具有密码的有效管理员")
	}
	var count int
	if e = tx.QueryRow(`SELECT count(*) FROM users WHERE access_role='system_admin'`).Scan(&count); e != nil {
		return e
	}
	if count != 0 {
		return fail(409, "CONFLICT", "已经存在系统管理员，请在站内管理权限")
	}
	if _, e = tx.Exec(`UPDATE users SET access_role='system_admin',can_edit=1 WHERE id=?`, id); e != nil {
		return e
	}
	if _, e = tx.Exec(`DELETE FROM sessions`); e != nil {
		return e
	}
	if e = audit(tx, id, "system.bootstrap", id); e != nil {
		return e
	}
	return tx.Commit()
}
func (a *App) RequireSystemAdmin() error {
	var n, count int
	if e := a.store.DB.QueryRow(`SELECT count(*) FROM users WHERE length(password_hash)>0`).Scan(&n); e != nil {
		return e
	}
	if e := a.store.DB.QueryRow(`SELECT count(*) FROM users WHERE access_role='system_admin' AND disabled=0 AND length(password_hash)>0`).Scan(&count); e != nil {
		return e
	}
	if n > 0 && count == 0 && !a.cfg.Demo {
		return fail(503, "SYSTEM_ADMIN_REQUIRED", "升级前请用 promote-system-admin --user-id 显式指定一位现有可登录管理员")
	}
	return nil
}
