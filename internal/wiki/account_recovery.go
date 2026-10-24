package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PrepareEmailRecovery is CLI-only. Server ownership authorizes a one-time
// recovery link; the recipient must still prove ownership of the new email.
func (a *App) PrepareEmailRecovery(id, output string) (string, error) {
	tx, e := a.store.DB.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var role string
	if e = tx.QueryRow(`SELECT access_role FROM users WHERE id=? AND disabled=0`, id).Scan(&role); e != nil || role != "system_admin" {
		return "", fmt.Errorf("recovery target must be an enabled system administrator")
	}
	token := newID()
	if _, e = tx.Exec(`UPDATE invites SET revoked_at=? WHERE recovery_user_id=? AND consumed_at='' AND revoked_at=''`, now(), id); e != nil {
		return "", e
	}
	if e = revokeCredentials(tx, id); e != nil {
		return "", e
	}
	if _, e = tx.Exec(`INSERT INTO invites(token_hash,role,expires_at,created_by,note,created_at,recovery_user_id) VALUES(?,?,?,?,?,?,?)`, hashToken(token), "user", time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), id, "Server recovery", now(), id); e != nil {
		return "", e
	}
	if e = audit(tx, id, "system.local_recovery", id); e != nil {
		return "", e
	}
	if e = os.MkdirAll(output, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(output, "email-recovery-"+newID()+".txt")
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	_, writeErr := f.WriteString(strings.TrimRight(a.cfg.BaseURL, "/") + "/account?invite=" + token + "\n")
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		return "", fmt.Errorf("cannot save private recovery link")
	}
	if e = tx.Commit(); e != nil {
		os.Remove(path)
		return "", e
	}
	return path, nil
}
