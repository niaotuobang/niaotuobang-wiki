package wiki

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (a *App) updateProfile(w http.ResponseWriter, r *http.Request, u *User) error {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	name := strings.TrimSpace(body.Nickname)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 40 || strings.ContainsFunc(name, unicode.IsControl) {
		return fail(400, "VALIDATION_ERROR", "昵称请填写 1 至 40 个字，不能包含换行或控制字符")
	}
	tx, err := a.store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE users SET name=? WHERE id=? AND disabled=0`, name, u.ID)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return fail(403, "FORBIDDEN", "账号已停用")
	}
	if err := audit(tx, u.ID, "account.profile.update", u.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]string{"name": name})
	return nil
}
