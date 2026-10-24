package wiki

import (
	"net/http"
	"time"
)

func (a *App) inviteRoutes() {
	a.route("POST /api/v1/admin/invites", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		var b struct {
			Role string `json:"role"`
		}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		if b.Role != "viewer" && b.Role != "editor" && b.Role != "user" {
			return fail(400, "VALIDATION_ERROR", "邀请只能创建普通成员，可选只读")
		}
		token := newID()
		tx, e := a.store.DB.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, e = tx.Exec(`INSERT INTO invites(token_hash,role,expires_at,created_by) VALUES(?,?,?,?)`, hashToken(token), b.Role, time.Now().UTC().Add(7*24*time.Hour).Format(time.RFC3339Nano), u.ID); e != nil {
			return e
		}
		if e = audit(tx, u.ID, "invite.create", hashToken(token)); e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		writeJSON(w, 201, map[string]string{"invite": token})
		return nil
	})
	a.route("GET /api/v1/admin/invites", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		cursor := r.URL.Query().Get("cursor")
		rows, e := a.store.DB.Query(`SELECT token_hash,role,expires_at,created_by,revoked_at,consumed_at FROM invites WHERE (?='' OR token_hash>?) ORDER BY token_hash LIMIT 21`, cursor, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, role, expiry, creator, revoked, consumed string
			if e = rows.Scan(&id, &role, &expiry, &creator, &revoked, &consumed); e != nil {
				return e
			}
			state := "active"
			if consumed != "" {
				state = "consumed"
			} else if revoked != "" {
				state = "revoked"
			} else if expiry < now() {
				state = "expired"
			}
			items = append(items, map[string]string{"id": id, "role": role, "expires_at": expiry, "created_by": creator, "state": state})
		}
		if e = rows.Err(); e != nil {
			return e
		}
		next := ""
		if len(items) > 20 {
			items = items[:20]
			next = items[19]["id"]
		}
		writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
		return nil
	})
	a.route("DELETE /api/v1/admin/invites/{id}", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		tx, e := a.store.DB.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, e = tx.Exec(`UPDATE invites SET revoked_at=? WHERE token_hash=? AND consumed_at='' AND revoked_at=''`, now(), r.PathValue("id")); e != nil {
			return e
		}
		if e = audit(tx, u.ID, "invite.revoke", r.PathValue("id")); e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
	a.route("GET /api/v1/system/audit", "system", func(w http.ResponseWriter, r *http.Request, u *User) error {
		rows, e := a.store.DB.Query(`SELECT id,actor_id,event,object_id,result,created_at FROM auth_audit_events ORDER BY created_at DESC,id LIMIT 50`)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, actor, event, object, result, date string
			if e = rows.Scan(&id, &actor, &event, &object, &result, &date); e != nil {
				return e
			}
			items = append(items, map[string]string{"id": id, "actor_id": actor, "event": event, "object_id": object, "result": result, "created_at": date})
		}
		if e = rows.Err(); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return nil
	})
}
