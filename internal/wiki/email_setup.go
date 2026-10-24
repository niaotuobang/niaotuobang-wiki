package wiki

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) setupWithEmail(w http.ResponseWriter, r *http.Request, _ *User) error {
	var b struct {
		codeProof
		Token string `json:"token"`
		Name  string `json:"name"`
	}
	if e := decode(w, r, &b); e != nil {
		return e
	}
	var u User
	var token string
	if e := a.withChallenge(r, b.codeProof, "setup", func(tx *sql.Tx, c challenge) error {
		if a.setupToken == "" || subtle.ConstantTimeCompare([]byte(a.setupToken), []byte(b.Token)) != 1 || c.InviteHash != hashToken(b.Token) || c.Kind != "email" {
			return fail(403, "FORBIDDEN", "初始化凭证无效")
		}
		name := strings.TrimSpace(b.Name)
		if name == "" || len([]rune(name)) > 40 {
			return fail(400, "VALIDATION_ERROR", "请填写昵称")
		}
		var count int
		if e := tx.QueryRow(`SELECT count(*) FROM users`).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			return fail(403, "FORBIDDEN", "初始化已完成")
		}
		u = User{ID: newID(), Name: name, Login: "member-" + newID(), Role: "system_admin", CanEdit: true}
		if e := insertUser(tx, u, []byte{}); e != nil {
			return e
		}
		if e := insertIdentity(tx, u.ID, c); e != nil {
			return e
		}
		var e error
		token, e = a.makeVerifiedSession(tx, u)
		if e != nil {
			return e
		}
		return audit(tx, u.ID, "system.setup", u.ID)
	}); e != nil {
		return e
	}
	a.authMu.Lock()
	a.setupToken = ""
	a.authMu.Unlock()
	os.Remove(filepath.Join(a.store.DataDir, "setup-token"))
	a.sessionResponse(w, u, token)
	return nil
}
